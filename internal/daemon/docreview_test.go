package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

func TestResolveDocPath(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	deck := filepath.Join(dir, "deck.pptx")
	if err := os.WriteFile(deck, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
	upperDeck := filepath.Join(dir, "DECK.PPTX")
	if err := os.WriteFile(upperDeck, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatalf("write upper deck: %v", err)
	}
	slides := filepath.Join(dir, "slides")
	if err := os.Mkdir(slides, 0o755); err != nil {
		t.Fatalf("mkdir slides: %v", err)
	}
	// A directory named like an unreadable format must still resolve —
	// the extension check applies to files only.
	dirWithExt := filepath.Join(dir, "bundle.pptx")
	if err := os.Mkdir(dirWithExt, 0o755); err != nil {
		t.Fatalf("mkdir bundle.pptx: %v", err)
	}

	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "absolute file", in: doc, want: doc},
		{name: "trimmed", in: "  " + doc + "  ", want: doc},
		{name: "directory of files", in: slides, want: slides},
		{name: "directory with a doc-like extension", in: dirWithExt, want: dirWithExt},
		{name: "empty", in: "   ", wantErr: "empty document path"},
		{name: "missing", in: filepath.Join(dir, "nope.md"), wantErr: "no such file"},
		{name: "unreadable format", in: deck, wantErr: "export it to PDF"},
		{name: "unreadable format uppercase extension", in: upperDeck, wantErr: "export it to PDF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDocPath(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveDocPath(%q) = %q, want error containing %q", tc.in, got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ResolveDocPath(%q) error = %v, want it to contain %q", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDocPath(%q): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveDocPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A relative path must resolve against the daemon's working directory,
// and `~` against the user's home — both are things a user types.
func TestResolveDocPathExpansions(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(doc, []byte("notes\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	t.Chdir(dir)
	got, err := ResolveDocPath("notes.md")
	if err != nil {
		t.Fatalf("relative path: unexpected error %v", err)
	}
	// macOS temp dirs are symlinked (/var -> /private/var), so compare
	// resolved forms rather than the raw strings.
	wantResolved, _ := filepath.EvalSymlinks(doc)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved {
		t.Fatalf("relative path = %q, want %q", gotResolved, wantResolved)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err = ResolveDocPath("~")
	if err != nil {
		t.Fatalf("ResolveDocPath(\"~\"): %v", err)
	}
	if got != home {
		t.Errorf("ResolveDocPath(\"~\") = %q, want %q", got, home)
	}
	// `~/x` must join against home, not concatenate the tilde away.
	got, err = ResolveDocPath("~/" + filepath.Base(home))
	if err == nil && got != filepath.Join(home, filepath.Base(home)) {
		t.Errorf("ResolveDocPath(\"~/<name>\") = %q, want %q", got, filepath.Join(home, filepath.Base(home)))
	}
}

// newDocTestServer builds an isolated daemon with no repos configured —
// a doc review needs none.
//
// The tmux session name MUST be a unique nonexistent one. Creation runs
// all the way to spawning Claude, and config.Default() names the user's
// real session ("tracks"), so a default-config server here would open
// live windows running real Claude processes in it. Pointing at a
// missing session makes the spawn fail instead, which is what these
// tests want anyway: failCreate then persists the track *and* its
// draft, which is the record under test.
func newDocTestServer(t *testing.T, repos ...config.Repo) (*Server, *state.MemoryStore) {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Repos = repos
	cfg.Tmux.SessionName = "tracks-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	// Checked, not assumed: if this name ever resolves, the spawn would
	// succeed and open live Claude windows in somebody's session. Fail
	// loudly instead.
	if tmux.New().HasSession(cfg.Tmux.SessionName) {
		t.Fatalf("test tmux session %q exists; refusing to run a spawn against a live session", cfg.Tmux.SessionName)
	}
	store := state.NewMemoryStore()
	return NewServer(cfg, store, "test"), store
}

func newDoc(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("# doc\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	return path
}

// createDoc runs handleNew with the given params and returns the single
// persisted track. The spawn failure is expected and irrelevant — the
// wiring under test all happens before it.
func createDoc(t *testing.T, srv *Server, store *state.MemoryStore, p NewParams) state.Track {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	srv.handleNew(context.Background(), raw, func(string) {})
	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 persisted track, got %d", len(tracks))
	}
	return tracks[0]
}

// A doc_path makes the track a doc review regardless of the kind the
// client sent, stores the resolved path, and derives the slug from the
// document's filename so the dashboard row and tmux tab aren't blank
// (a doc track has no branch to fall back on).
func TestNewDocTrackWiring(t *testing.T) {
	srv, store := newDocTestServer(t)
	doc := newDoc(t, "architecture-review.md")

	// Sent relative, on purpose: storing the raw value would leave
	// DocPath unusable from the daemon's own working directory.
	t.Chdir(filepath.Dir(doc))
	tr := createDoc(t, srv, store, NewParams{
		TaskPrompt: "review it",
		DocPath:    "architecture-review.md",
		Kind:       "work", // must lose to the doc path
	})

	if tr.Kind != state.KindDoc {
		t.Errorf("Kind = %q, want doc even though the client sent work", tr.Kind)
	}
	if tr.DocPath != doc {
		t.Errorf("DocPath = %q, want the resolved absolute %q", tr.DocPath, doc)
	}
	if tr.Slug != "architecture-review" {
		t.Errorf("Slug = %q, want it derived from the document filename", tr.Slug)
	}
	if tr.Branch != "" {
		t.Errorf("Branch = %q, want empty on a worktree-less doc track", tr.Branch)
	}
}

// An explicit slug wins over the filename derivation.
func TestNewDocTrackKeepsExplicitSlug(t *testing.T) {
	srv, store := newDocTestServer(t)
	tr := createDoc(t, srv, store, NewParams{
		TaskPrompt: "review it",
		DocPath:    newDoc(t, "deck.pdf"),
		Slug:       "q3-deck",
	})
	if tr.Slug != "q3-deck" {
		t.Errorf("Slug = %q, want the explicit q3-deck", tr.Slug)
	}
}

// The draft must carry the *resolved* path: a relative one would
// re-resolve against the daemon's cwd on a later relaunch. And the
// draft's kind has to survive the round-trip into NewParams.
func TestNewDocTrackDraftRoundTrip(t *testing.T) {
	srv, store := newDocTestServer(t)
	doc := newDoc(t, "spec.md")
	t.Chdir(filepath.Dir(doc))
	tr := createDoc(t, srv, store, NewParams{TaskPrompt: "review it", DocPath: "spec.md"})

	if tr.Draft == nil {
		t.Fatal("failed creation should capture a draft")
	}
	if tr.Draft.DocPath != doc {
		t.Errorf("Draft.DocPath = %q, want the resolved %q", tr.Draft.DocPath, doc)
	}
	if tr.Draft.Kind != string(state.KindDoc) {
		t.Errorf("Draft.Kind = %q, want doc", tr.Draft.Kind)
	}

	// The relaunch path reconstructs NewParams from the draft; a dropped
	// DocPath there would silently turn a doc review into a work track.
	params := NewParams{
		Repos:      tr.Draft.Repos,
		TaskPrompt: tr.Draft.TaskPrompt,
		Slug:       tr.Draft.Slug,
		ReviewRef:  tr.Draft.ReviewRef,
		DocPath:    tr.Draft.DocPath,
		Kind:       tr.Draft.Kind,
	}
	if params.DocPath != doc {
		t.Errorf("relaunch params lost DocPath: %+v", params)
	}
}

// The review shape (candor + section switches) has to reach both the
// track and its draft, or a relaunch would silently run a different
// review than the one the user configured.
func TestNewDocTrackCarriesReviewShape(t *testing.T) {
	srv, store := newDocTestServer(t)
	tr := createDoc(t, srv, store, NewParams{
		TaskPrompt:        "review it",
		DocPath:           newDoc(t, "spec.md"),
		Candor:            8,
		DocSkipClaimCheck: true,
	})

	if tr.Candor != 8 {
		t.Errorf("Candor = %d, want 8", tr.Candor)
	}
	if !tr.DocSkipClaimCheck {
		t.Error("DocSkipClaimCheck lost on the track")
	}
	if tr.DocSkipOpinion {
		t.Error("DocSkipOpinion set when the client didn't ask for it")
	}
	if tr.Draft == nil {
		t.Fatal("failed creation should capture a draft")
	}
	if tr.Draft.Candor != 8 || !tr.Draft.DocSkipClaimCheck || tr.Draft.DocSkipOpinion {
		t.Errorf("draft lost the review shape: %+v", tr.Draft)
	}
}

// Candor only means something to a review, and the doc section switches
// only to a doc review. Storing them on other kinds would leave a work
// track carrying settings nothing reads.
func TestNewClearsReviewShapeOnOtherKinds(t *testing.T) {
	srv, store := newDocTestServer(t)
	raw, err := json.Marshal(NewParams{
		TaskPrompt:        "answer it",
		Kind:              "ask",
		Candor:            9,
		DocSkipOpinion:    true,
		DocSkipClaimCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.handleNew(context.Background(), raw, func(string) {})
	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 persisted track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Candor != 0 || tr.DocSkipOpinion || tr.DocSkipClaimCheck {
		t.Errorf("ask track kept review shape: candor=%d skipOpinion=%v skipClaimCheck=%v",
			tr.Candor, tr.DocSkipOpinion, tr.DocSkipClaimCheck)
	}
}

// An out-of-range candor is rejected at creation rather than clamped:
// the only way to send one is a buggy client, and silently reinterpreting
// it would hide that.
func TestNewRejectsOutOfRangeCandor(t *testing.T) {
	for _, candor := range []int{-1, 11, 99} {
		srv, store := newDocTestServer(t)
		raw, err := json.Marshal(NewParams{
			TaskPrompt: "review it",
			DocPath:    newDoc(t, "spec.md"),
			Candor:     candor,
		})
		if err != nil {
			t.Fatal(err)
		}
		resp := srv.handleNew(context.Background(), raw, func(string) {})
		if resp.Ok || !strings.Contains(resp.Error, "candor must be between") {
			t.Errorf("candor %d: want a range rejection, got ok=%v err=%q", candor, resp.Ok, resp.Error)
		}
		if n := len(store.All()); n != 0 {
			t.Errorf("candor %d: persisted %d tracks for a rejected creation", candor, n)
		}
	}
}

func TestNewDocTrackRejections(t *testing.T) {
	cases := []struct {
		name string
		// docPath is a filename created for the case; when empty, rawPath
		// is sent as-is (for the missing-file and no-path cases).
		docPath string
		rawPath string
		kind    string
		wantErr string
	}{
		{
			name:    "kind doc with no path",
			kind:    "doc",
			wantErr: "needs a document path",
		},
		{
			name:    "missing file",
			rawPath: "/nonexistent/deck.pdf",
			wantErr: "no such file",
		},
		{
			// An existing file whose format Claude can't read: the
			// deny-list must reject it, not os.Stat.
			name:    "unreadable format",
			docPath: "deck.pptx",
			wantErr: "export it to PDF",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, store := newDocTestServer(t)
			path := tc.rawPath
			if tc.docPath != "" {
				path = newDoc(t, tc.docPath)
			}
			raw, err := json.Marshal(NewParams{TaskPrompt: "x", Kind: tc.kind, DocPath: path})
			if err != nil {
				t.Fatal(err)
			}
			resp := srv.handleNew(context.Background(), raw, func(string) {})
			if resp.Ok {
				t.Fatal("expected the creation to be rejected")
			}
			if !strings.Contains(resp.Error, tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", resp.Error, tc.wantErr)
			}
			// Rejected before any record exists — nothing to clean up.
			if n := len(store.All()); n != 0 {
				t.Errorf("persisted %d tracks for a rejected creation, want 0", n)
			}
		})
	}
}

// A doc path and a review ref are mutually exclusive targets; accepting
// both would silently ignore one of them (and leave it in the draft).
func TestNewRejectsDocPathWithReviewRef(t *testing.T) {
	srv, _ := newDocTestServer(t, config.Repo{Name: "demo", Path: "/nonexistent/demo", Base: "main"})

	raw, err := json.Marshal(NewParams{
		Repos:      []string{"demo"},
		TaskPrompt: "x",
		DocPath:    newDoc(t, "spec.md"),
		ReviewRef:  "feat/foo",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(context.Background(), raw, func(string) {})
	if resp.Ok || !strings.Contains(resp.Error, "not both") {
		t.Errorf("want a both-targets rejection, got ok=%v err=%q", resp.Ok, resp.Error)
	}
}

// Promotion is for ask/plan: a doc review has no code work to promote
// to, and allowing it would strand DocPath on a work-kind record.
func TestPromoteRejectsDocTrack(t *testing.T) {
	srv, store := newDocTestServer(t)
	tr := createDoc(t, srv, store, NewParams{TaskPrompt: "review it", DocPath: newDoc(t, "spec.md")})

	raw, err := json.Marshal(PromoteParams{ID: tr.ID})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handlePromote(context.Background(), raw, func(string) {})
	if resp.Ok || !strings.Contains(resp.Error, "can't be promoted") {
		t.Errorf("want a promote rejection for a doc track, got ok=%v err=%q", resp.Ok, resp.Error)
	}
}
