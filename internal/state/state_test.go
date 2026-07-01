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
	for _, k := range []Kind{KindAsk, KindPlan} {
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
		StatusPending: false,
		StatusRunning: false,
		StatusWaiting: false,
		StatusPR:      false,
		StatusDone:    true,
		StatusErrored: true,
	}
	for s, want := range cases {
		if s.IsTerminal() != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, !want, want)
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
