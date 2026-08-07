package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeTrack(id string) Track {
	return Track{
		ID:         id,
		Branch:     "fix/example",
		Repos:      []TrackRepo{{Name: "demo-repo", Path: "/tmp/" + id + "/demo-repo"}},
		Status:     StatusRunning,
		LogPath:    "/tmp/" + id + ".jsonl",
		TaskPrompt: "do the thing",
	}
}

func TestKindWorktreeless(t *testing.T) {
	for _, k := range []Kind{KindAsk, KindPlan, KindDoc} {
		if !k.Worktreeless() {
			t.Errorf("%q should be worktreeless", k)
		}
	}
	for _, k := range []Kind{KindWork, KindReview, Kind("")} {
		if k.Worktreeless() {
			t.Errorf("%q should not be worktreeless", k)
		}
	}
}

// KIND is rendered into a 7-wide dashboard column; a longer value
// would break the table's alignment for every row.
func TestKindFitsDashboardColumn(t *testing.T) {
	for _, k := range []Kind{KindWork, KindReview, KindAsk, KindPlan, KindDoc} {
		if len(k) > 7 {
			t.Errorf("kind %q is %d chars, want <= 7 (dashboard KIND column width)", k, len(k))
		}
	}
}

func TestDocDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(file, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := (Track{}).DocDir(); got != "" {
		t.Errorf("DocDir() with no DocPath = %q, want empty", got)
	}
	if got := (Track{DocPath: file}).DocDir(); got != dir {
		t.Errorf("DocDir() for a file = %q, want its parent %q", got, dir)
	}
	if got := (Track{DocPath: dir}).DocDir(); got != dir {
		t.Errorf("DocDir() for a directory = %q, want %q", got, dir)
	}
	// A document deleted after track creation must not break spawning:
	// fall back to the parent so Claude reports the missing file itself.
	gone := filepath.Join(dir, "vanished", "deck.pdf")
	if got := (Track{DocPath: gone}).DocDir(); got != filepath.Dir(gone) {
		t.Errorf("DocDir() for a missing path = %q, want %q", got, filepath.Dir(gone))
	}
}

func TestKindRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	tr := makeTrack("a")
	tr.Kind = KindPlan
	if err := fs.Put(tr); err != nil {
		t.Fatal(err)
	}
	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := fs2.Get("a")
	if got.Kind != KindPlan {
		t.Errorf("kind = %q, want plan", got.Kind)
	}
}

func TestMigrateV1TracksGetKind(t *testing.T) {
	dir := t.TempDir()
	// A schema-v1 file: tracks have no `kind`. The plain branch should
	// migrate to work; a pr/ branch (left by a review track) to review.
	raw := `{"schema_version":1,"tracks":[` +
		`{"id":"w","branch":"fix/x","repos":[],"status":"done","log_path":"","task_prompt":""},` +
		`{"id":"r","branch":"pr/123","repos":[],"status":"done","log_path":"","task_prompt":""}` +
		`]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := fs.Get("w"); w.Kind != KindWork {
		t.Errorf("plain v1 track kind = %q, want work", w.Kind)
	}
	if r, _ := fs.Get("r"); r.Kind != KindReview {
		t.Errorf("pr/ v1 track kind = %q, want review", r.Kind)
	}
}

// A v2 file carries one PR in flat pr_* fields and spells the in-review
// status "pr". Both must survive the upgrade: the PR becomes PRs[0] with
// its polled state intact, and the status becomes "pr open".
func TestMigrateV2SinglePRBecomesPRList(t *testing.T) {
	dir := t.TempDir()
	raw := `{"schema_version":2,"tracks":[` +
		`{"id":"a","branch":"fix/x","kind":"work","repos":[],"status":"pr",` +
		`"log_path":"","task_prompt":"",` +
		`"pr_url":"https://example.test/pr/7","pr_state":"OPEN",` +
		`"pr_draft":true,"pr_review_state":"CHANGES_REQUESTED","pr_comments":3},` +
		`{"id":"b","branch":"fix/y","kind":"work","repos":[],"status":"done",` +
		`"log_path":"","task_prompt":""}` +
		`]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := fs.Get("a")
	if a.Status != StatusPROpen {
		t.Errorf("status = %q, want %q", a.Status, StatusPROpen)
	}
	if len(a.PRs) != 1 {
		t.Fatalf("PRs = %+v, want exactly one entry", a.PRs)
	}
	want := PRRef{
		URL:         "https://example.test/pr/7",
		State:       "OPEN",
		Draft:       true,
		ReviewState: "CHANGES_REQUESTED",
		Comments:    3,
	}
	if a.PRs[0] != want {
		t.Errorf("PRs[0] = %+v, want %+v", a.PRs[0], want)
	}
	// A track that never opened a PR must not gain a phantom entry.
	if b, _ := fs.Get("b"); len(b.PRs) != 0 {
		t.Errorf("PR-less track migrated to %+v, want none", b.PRs)
	}
}

func TestPRListRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	prs := []PRRef{
		{URL: "https://example.test/pr/1", State: "MERGED"},
		{URL: "https://example.test/pr/2", State: "OPEN", Comments: 2},
	}
	if err := fs.Put(Track{ID: "s", Status: StatusPROpen, PRs: prs}); err != nil {
		t.Fatal(err)
	}
	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := fs2.Get("s")
	if len(got.PRs) != 2 || got.PRs[0] != prs[0] || got.PRs[1] != prs[1] {
		t.Errorf("PRs = %+v, want %+v", got.PRs, prs)
	}
}

func TestStatusLabel(t *testing.T) {
	pr := func(state string) PRRef { return PRRef{URL: "u", State: state} }
	cases := []struct {
		name string
		trk  Track
		want string
	}{
		{"no PR", Track{Status: StatusDone}, "done"},
		{"one open", Track{Status: StatusPROpen, PRs: []PRRef{pr("OPEN")}}, "pr open"},
		{"two, one open", Track{Status: StatusPROpen,
			PRs: []PRRef{pr("MERGED"), pr("OPEN")}}, "prs open"},
		{"one merged", Track{Status: StatusPRMerged, PRs: []PRRef{pr("MERGED")}}, "pr merged"},
		{"three merged", Track{Status: StatusPRMerged,
			PRs: []PRRef{pr("MERGED"), pr("MERGED"), pr("MERGED")}}, "all merged"},
		{"running", Track{Status: StatusRunning}, "running"},
		{"interrupted", Track{Status: StatusInterrupted}, "interrupted"},
	}
	for _, c := range cases {
		if got := c.trk.StatusLabel(); got != c.want {
			t.Errorf("%s: StatusLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}

// The dashboard's STATUS column is fixed-width; a label wider than it
// would be silently truncated mid-word.
func TestStatusLabelsFitDashboardColumn(t *testing.T) {
	const statusColWidth = 11
	labels := []string{
		string(StatusPending), string(StatusRunning), string(StatusWaiting),
		string(StatusDone), string(StatusErrored), string(StatusInterrupted),
		string(StatusDraft), "pr open", "prs open", "pr merged", "all merged",
	}
	for _, l := range labels {
		if len(l) > statusColWidth {
			t.Errorf("status label %q is %d chars, wider than the %d-char column",
				l, len(l), statusColWidth)
		}
	}
}

func TestTrackPRHelpers(t *testing.T) {
	var trk Track
	if !trk.AddPR("https://example.test/pr/1") {
		t.Error("AddPR on an empty track = false, want true")
	}
	if trk.AddPR("https://example.test/pr/1") {
		t.Error("AddPR of a known URL = true, want false (no duplicate)")
	}
	if trk.AddPR("") {
		t.Error("AddPR(\"\") = true, want false")
	}
	trk.AddPR("https://example.test/pr/2")
	if got := trk.PRIndex("https://example.test/pr/2"); got != 1 {
		t.Errorf("PRIndex = %d, want 1", got)
	}
	if got := trk.PRIndex("nope"); got != -1 {
		t.Errorf("PRIndex of unknown URL = %d, want -1", got)
	}
	// Unpolled PRs count as open, so nothing is merged yet.
	if !trk.HasOpenPR() || trk.AllPRsMerged() {
		t.Errorf("unpolled PRs: HasOpenPR=%v AllPRsMerged=%v, want true/false",
			trk.HasOpenPR(), trk.AllPRsMerged())
	}
	trk.SetPR(0, PRRef{URL: trk.PRs[0].URL, State: "MERGED"})
	trk.SetPR(1, PRRef{URL: trk.PRs[1].URL, State: "MERGED"})
	if trk.HasOpenPR() || !trk.AllPRsMerged() {
		t.Errorf("both merged: HasOpenPR=%v AllPRsMerged=%v, want false/true",
			trk.HasOpenPR(), trk.AllPRsMerged())
	}
	if got := trk.MergedPRs(); got != 2 {
		t.Errorf("MergedPRs = %d, want 2", got)
	}
	// SetPR must not touch a snapshot taken before the write — the two
	// Tracks share a backing array until the copy-on-write kicks in.
	snapshot := trk
	if !trk.SetPR(0, PRRef{URL: trk.PRs[0].URL, State: "CLOSED"}) {
		t.Error("SetPR of a changed entry = false, want true")
	}
	if snapshot.PRs[0].State != "MERGED" {
		t.Errorf("SetPR wrote through to an earlier snapshot: %+v", snapshot.PRs[0])
	}
	if trk.SetPR(0, trk.PRs[0]) {
		t.Error("SetPR of an unchanged entry = true, want false")
	}
	if trk.SetPR(9, PRRef{URL: "x"}) {
		t.Error("SetPR out of range = true, want false")
	}
}

func TestPRRefNumber(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r/pull/123": "#123",
		"https://example.test/pr/7":       "#7",
		"https://github.com/o/r/pull/":    "",
		"not-a-url":                       "",
		"https://github.com/o/r/pull/abc": "",
	}
	for url, want := range cases {
		if got := (PRRef{URL: url}).Number(); got != want {
			t.Errorf("Number(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestFileStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("a")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("b")); err != nil {
		t.Fatal(err)
	}
	// Re-open from disk and check that both tracks survived.
	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := fs2.All()
	if len(all) != 2 {
		t.Fatalf("got %d tracks, want 2", len(all))
	}
	if _, ok := fs2.Get("b"); !ok {
		t.Error("track b missing after reload")
	}
}

func TestFileStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("a")); err != nil {
		t.Fatal(err)
	}
	// No stray temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, e := range entries {
		if e.Name() == "state.json" {
			got++
			continue
		}
		t.Errorf("unexpected file in state dir: %s", e.Name())
	}
	if got != 1 {
		t.Errorf("state.json not found in %s", dir)
	}
}

func TestFileStoreDelete(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = fs.Put(makeTrack("a"))
	existed, err := fs.Delete("a")
	if err != nil || !existed {
		t.Fatalf("Delete: existed=%v err=%v", existed, err)
	}
	existed, err = fs.Delete("a")
	if err != nil || existed {
		t.Fatalf("Delete idempotent: existed=%v err=%v", existed, err)
	}
}

func TestFileStoreSortedByCreatedAt(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t1 := makeTrack("first")
	t1.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := makeTrack("second")
	t2.CreatedAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	// Insert in reverse order.
	_ = fs.Put(t2)
	_ = fs.Put(t1)
	all := fs.All()
	if all[0].ID != "first" || all[1].ID != "second" {
		t.Errorf("not sorted by CreatedAt: %v", []string{all[0].ID, all[1].ID})
	}
}

func TestFileStoreRejectsEmptyID(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(Track{ID: ""}); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestFileStoreFutureSchemaRefused(t *testing.T) {
	dir := t.TempDir()
	bogus := map[string]any{
		"schema_version": CurrentSchemaVersion + 1,
		"tracks":         []any{},
	}
	data, _ := json.Marshal(bogus)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(dir); err == nil {
		t.Fatal("expected error opening future-schema state")
	}
}

func TestMemoryStoreImplementsStoreContract(t *testing.T) {
	var s Store = NewMemoryStore()
	_ = s.Put(makeTrack("x"))
	if got, ok := s.Get("x"); !ok || got.ID != "x" {
		t.Fatalf("Get(x): %+v ok=%v", got, ok)
	}
	if len(s.All()) != 1 {
		t.Fatalf("All: %d", len(s.All()))
	}
	if existed, err := s.Delete("x"); err != nil || !existed {
		t.Fatalf("Delete: existed=%v err=%v", existed, err)
	}
}

// updateStores runs a Store.Update assertion against both implementations
// so the two stay behaviourally identical.
func updateStores(t *testing.T) map[string]Store {
	t.Helper()
	fs, err := OpenFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return map[string]Store{"memory": NewMemoryStore(), "file": fs}
}

func TestUpdateMutatesAndPersists(t *testing.T) {
	for name, s := range updateStores(t) {
		t.Run(name, func(t *testing.T) {
			_ = s.Put(makeTrack("a"))
			got, found, err := s.Update("a", func(tr *Track) bool {
				tr.Status = StatusWaiting
				return true
			})
			if err != nil || !found {
				t.Fatalf("Update: found=%v err=%v", found, err)
			}
			if got.Status != StatusWaiting {
				t.Errorf("returned track not updated: %v", got.Status)
			}
			if reread, _ := s.Get("a"); reread.Status != StatusWaiting {
				t.Errorf("persisted track not updated: %v", reread.Status)
			}
		})
	}
}

func TestUpdateOnlyTouchesClosureFields(t *testing.T) {
	// The point of Update: a writer that only sets Status must not drop a
	// field (Services) another writer set — the lost-update Get+Put risks.
	for name, s := range updateStores(t) {
		t.Run(name, func(t *testing.T) {
			base := makeTrack("a")
			base.Services = []ServiceState{{Name: "web", Status: ServiceReady, PGID: 4242}}
			_ = s.Put(base)

			if _, _, err := s.Update("a", func(tr *Track) bool {
				tr.Status = StatusWaiting
				return true
			}); err != nil {
				t.Fatal(err)
			}
			got, _ := s.Get("a")
			if len(got.Services) != 1 || got.Services[0].PGID != 4242 {
				t.Errorf("Services clobbered by a Status-only update: %+v", got.Services)
			}
		})
	}
}

func TestUpdateNoChangeDoesNotBumpUpdatedAt(t *testing.T) {
	for name, s := range updateStores(t) {
		t.Run(name, func(t *testing.T) {
			_ = s.Put(makeTrack("a"))
			before, _ := s.Get("a")
			time.Sleep(2 * time.Millisecond)
			got, found, err := s.Update("a", func(tr *Track) bool { return false })
			if err != nil || !found {
				t.Fatalf("Update: found=%v err=%v", found, err)
			}
			if !got.UpdatedAt.Equal(before.UpdatedAt) {
				t.Errorf("UpdatedAt bumped despite no change: %v -> %v", before.UpdatedAt, got.UpdatedAt)
			}
		})
	}
}

func TestUpdateUnknownIDReturnsFalse(t *testing.T) {
	for name, s := range updateStores(t) {
		t.Run(name, func(t *testing.T) {
			called := false
			_, found, err := s.Update("ghost", func(tr *Track) bool {
				called = true
				return true
			})
			if err != nil {
				t.Fatal(err)
			}
			if found {
				t.Error("expected found=false for unknown id")
			}
			if called {
				t.Error("mutate must not be called for unknown id")
			}
		})
	}
}

func TestUpdatePersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = fs.Put(makeTrack("a"))
	if _, _, err := fs.Update("a", func(tr *Track) bool {
		tr.Status = StatusDone
		return true
	}); err != nil {
		t.Fatal(err)
	}
	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := fs2.Get("a"); got.Status != StatusDone {
		t.Errorf("update not flushed to disk: %v", got.Status)
	}
}

func TestStatusIsTerminal(t *testing.T) {
	cases := map[Status]bool{
		StatusPending:     false,
		StatusRunning:     false,
		StatusWaiting:     false,
		StatusPROpen:      false,
		StatusDone:        true,
		StatusPRMerged:    true,
		StatusErrored:     true,
		StatusInterrupted: true,
	}
	for s, want := range cases {
		if s.IsTerminal() != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, !want, want)
		}
	}
}

// Completed is the narrower predicate prune / gc use: an interrupted
// track is terminal but the user still means to reopen it, so it must
// never count as completed.
func TestStatusCompleted(t *testing.T) {
	cases := map[Status]bool{
		StatusPending:     false,
		StatusRunning:     false,
		StatusWaiting:     false,
		StatusPROpen:      false,
		StatusDraft:       false,
		StatusDone:        true,
		StatusPRMerged:    true,
		StatusErrored:     true,
		StatusInterrupted: false,
	}
	for s, want := range cases {
		if s.Completed() != want {
			t.Errorf("%s.Completed() = %v, want %v", s, !want, want)
		}
	}
}

func TestTrackResumable(t *testing.T) {
	const sid = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	cases := []struct {
		name string
		trk  Track
		want bool
	}{
		{"running", Track{Status: StatusRunning, SessionID: sid}, false},
		{"in review", Track{Status: StatusPROpen, SessionID: sid}, false},
		{"draft", Track{Status: StatusDraft, SessionID: sid}, false},
		{"done", Track{Status: StatusDone, SessionID: sid}, true},
		{"pr merged", Track{Status: StatusPRMerged, SessionID: sid}, true},
		{"errored", Track{Status: StatusErrored, SessionID: sid}, true},
		{"interrupted", Track{Status: StatusInterrupted, SessionID: sid}, true},
		{"interrupted without session", Track{Status: StatusInterrupted}, false},
	}
	for _, c := range cases {
		if got := c.trk.Resumable(); got != c.want {
			t.Errorf("%s: Resumable() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTrackWindowName(t *testing.T) {
	cases := []struct {
		name string
		trk  Track
		want string
	}{
		{
			name: "slug drives the label",
			trk:  Track{ID: "20260624-101500-a1b2c3", Slug: "rate-bug", TaskPrompt: "investigate the rate spike"},
			want: "rate-bug-a1b2c3",
		},
		{
			name: "slug is sanitised to a tmux-safe token",
			trk:  Track{ID: "20260624-101500-a1b2c3", Slug: "Rate Bug: swap.v2!"},
			want: "rate-bug-swap-v2-a1b2c3",
		},
		{
			name: "falls back to the prompt when slug is empty",
			trk:  Track{ID: "20260624-101500-a1b2c3", TaskPrompt: "Investigate the rate spike on swap"},
			want: "investigate-the-rate-spi-a1b2c3",
		},
		{
			name: "falls back to t- form when slug and prompt are empty",
			trk:  Track{ID: "20260624-101500-a1b2c3"},
			want: "t-a1b2c3",
		},
		{
			name: "short id is used whole",
			trk:  Track{ID: "abc", Slug: "tiny"},
			want: "tiny-abc",
		},
		{
			name: "slug of only punctuation falls through to the prompt",
			trk:  Track{ID: "20260624-101500-a1b2c3", Slug: "!!!", TaskPrompt: "fix it"},
			want: "fix-it-a1b2c3",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.trk.WindowName(); got != c.want {
				t.Fatalf("WindowName() = %q, want %q", got, c.want)
			}
		})
	}
}

// Two tracks that share a slug must still get distinct window names,
// or the daemon could select/kill the wrong tmux window.
func TestTrackWindowNameUniquePerID(t *testing.T) {
	a := Track{ID: "20260624-101500-a1b2c3", Slug: "rate-bug"}
	b := Track{ID: "20260624-101501-d4e5f6", Slug: "rate-bug"}
	if a.WindowName() == b.WindowName() {
		t.Fatalf("expected distinct window names, both were %q", a.WindowName())
	}
}
