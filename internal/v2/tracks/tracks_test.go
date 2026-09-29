package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

var errStep = errors.New("step failed")

type failingStore struct {
	*store.Store
	fail bool
}

func (s *failingStore) AddTrack(ctx context.Context, t track.Track) error {
	if s.fail {
		return errStep
	}
	return s.Store.AddTrack(ctx, t)
}

func (s *failingStore) ReopenTrack(ctx context.Context, id, name string) error {
	if s.fail {
		return errStep
	}
	return s.Store.ReopenTrack(ctx, id, name)
}

type fakeWorktrees struct {
	fail, failRestore bool
	removed           []string // Remove's tracks
	cleaned           []string // RemoveWorktrees' tracks, when it had some
	unsaved           []workspace.Unsaved
	renamed           map[string]string // repo name to branch
	gone              map[string]bool   // repos whose worktree is gone
}

func (w *fakeWorktrees) Add(_ context.Context, t track.Track, progress func(string)) ([]track.Repo, error) {
	if w.fail {
		return nil, errStep
	}
	progress("Fetching…")
	repos := slices.Clone(t.Repos)
	if t.Kind.Worktrees() {
		for i := range repos {
			repos[i].Worktree, repos[i].Branch = "/wt/"+t.ID+"/"+repos[i].Name, track.Branch(t.ID)
		}
	}
	return repos, nil
}

func (w *fakeWorktrees) Remove(_ context.Context, t track.Track) error {
	w.removed = append(w.removed, t.ID)
	return nil
}

func (w *fakeWorktrees) Missing(t track.Track) []track.Repo {
	var out []track.Repo
	for _, r := range t.Repos {
		if w.gone[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

func (w *fakeWorktrees) Restore(_ context.Context, t track.Track, progress func(string)) ([]track.Repo, error) {
	if w.failRestore {
		return nil, errStep
	}
	made := w.Missing(t)
	for _, r := range made {
		progress("Re-creating the worktree for " + r.Name + "…")
	}
	return made, nil
}

func (w *fakeWorktrees) Unsaved(context.Context, track.Track) ([]workspace.Unsaved, error) {
	return w.unsaved, nil
}

// Branches puts the repos named in renamed on their new branch.
func (w *fakeWorktrees) Branches(_ context.Context, t track.Track) []track.Repo {
	repos := slices.Clone(t.Repos)
	for i, r := range repos {
		if b, ok := w.renamed[r.Name]; ok {
			repos[i].Branch = b
		}
	}
	return repos
}

func (w *fakeWorktrees) RemoveWorktrees(_ context.Context, id string, repos []track.Repo) error {
	if len(repos) > 0 {
		w.cleaned = append(w.cleaned, id)
	}
	return nil
}

type fakeWindows struct {
	windows []trackwin.Info
	opened  []trackwin.Spec
	closed  []string
	fail    bool
}

func (w *fakeWindows) List() ([]trackwin.Info, error) { return w.windows, nil }

func (w *fakeWindows) Open(s trackwin.Spec) (trackwin.Window, error) {
	w.opened = append(w.opened, s)
	id := "@" + s.Name
	if w.fail {
		// The window exists but setting it up failed.
		return trackwin.Window{ID: id}, errStep
	}
	w.windows = append(w.windows, trackwin.Info{Number: len(w.windows) + 1, Window: id, Track: s.Track, Name: s.Name})
	return trackwin.Window{ID: id, Agent: "%1"}, nil
}

func (w *fakeWindows) Close(window string) error {
	w.closed = append(w.closed, window)
	w.windows = slices.DeleteFunc(w.windows, func(in trackwin.Info) bool { return in.Window == window })
	return nil
}

type fakeEngine struct {
	failSession, failCommand bool
	spec                     agents.Spec
}

func (e *fakeEngine) Session(context.Context, string) (string, error) {
	if e.failSession {
		return "", errStep
	}
	return "session-1", nil
}

func (e *fakeEngine) Command(s agents.Spec) (agents.Start, error) {
	e.spec = s
	if e.failCommand {
		return agents.Start{}, errStep
	}
	return agents.Start{Command: "claude 'go'", Dir: "/start"}, nil
}

type fixture struct {
	svc       *Service
	store     *failingStore
	worktrees *fakeWorktrees
	windows   *fakeWindows
	engine    *fakeEngine
	engines   settings.Engines
	types     settings.Tracks
}

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "tracks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f := &fixture{
		store: &failingStore{Store: st}, worktrees: &fakeWorktrees{}, windows: &fakeWindows{}, engine: &fakeEngine{},
		engines: settings.Engines{Claude: &settings.Engine{Model: "opus"}},
	}
	for _, r := range []store.Repo{{Name: "api", BaseBranch: "main", DraftPRs: true}, {Name: "web", BaseBranch: "develop"}} {
		r.Path = filepath.Join(dir, r.Name)
		if err := os.Mkdir(r.Path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := st.AddRepo(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	f.svc = &Service{
		Store: f.store, Worktrees: f.worktrees, Windows: f.windows,
		Settings: func() (settings.Settings, error) { return settings.Settings{Engines: f.engines, Tracks: f.types}, nil },
		Engines:  map[string]Engine{"claude": f.engine, "cursor": f.engine},
		Now:      func() time.Time { return time.UnixMilli(1_790_000_000_000) },
		NewID: func() string {
			n++
			return "20260928-101500-abc12" + string(rune('0'+n))
		},
		SocketDir: "/data", BinDir: "/data/bin",
	}
	return f
}

func (f *fixture) saved(t *testing.T) []track.Track {
	open, err := f.store.OpenTracks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return open
}

func TestCreateWork(t *testing.T) {
	f := newFixture(t)
	var steps []string
	got, err := f.svc.Create(context.Background(), Request{
		Kind: track.Work, Repos: []string{"api", "web"}, Prompt: "Fix the login redirect", Terminal: true,
	}, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}

	tr := got.Track
	if tr.Name != "fix-the-login-redirect" || tr.Engine != "claude" || tr.Model != "opus" || tr.Session != "session-1" {
		t.Errorf("track = %+v", tr)
	}
	if len(tr.Repos) != 2 || tr.Repos[0].Worktree == "" || tr.Repos[1].Base != "develop" {
		t.Errorf("repos = %+v", tr.Repos)
	}
	spec := f.windows.opened[0]
	if spec.Track != tr.ID || spec.Kind != "work" || spec.Repo != "api,web" || spec.Dir != "/start" ||
		spec.Agent != (trackwin.Process{Title: "Claude Code", Command: "claude 'go'"}) || spec.Terminals != 1 {
		t.Errorf("window = %+v", spec)
	}
	if e := f.engine.spec; !e.Auto || !slices.Equal(e.DraftPRs, []string{"api"}) || e.Program != "claude" || e.BinDir != "/data/bin" {
		t.Errorf("engine spec = %+v", e)
	}
	if !slices.Equal(steps, []string{"Fetching…", "Starting Claude Code…"}) {
		t.Errorf("progress = %q", steps)
	}
	if saved := f.saved(t); len(saved) != 1 || saved[0].ID != tr.ID || saved[0].Repos[0].Worktree != tr.Repos[0].Worktree {
		t.Errorf("saved = %+v", saved)
	}
}

func TestCreateRollsBack(t *testing.T) {
	for _, c := range []struct {
		step           string
		fail           func(*fixture)
		removed, close bool
	}{
		{"session", func(f *fixture) { f.engine.failSession = true }, false, false},
		{"worktrees", func(f *fixture) { f.worktrees.fail = true }, false, false},
		{"command", func(f *fixture) { f.engine.failCommand = true }, true, false},
		{"window", func(f *fixture) { f.windows.fail = true }, true, true},
		{"save", func(f *fixture) { f.store.fail = true }, true, true},
	} {
		t.Run(c.step, func(t *testing.T) {
			f := newFixture(t)
			c.fail(f)
			_, err := f.svc.Create(context.Background(), Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
			if !errors.Is(err, errStep) {
				t.Fatalf("err = %v", err)
			}
			if got := len(f.worktrees.removed) == 1; got != c.removed {
				t.Errorf("worktrees removed: %v, want %v", got, c.removed)
			}
			if got := len(f.windows.closed) == 1; got != c.close {
				t.Errorf("window closed: %v, want %v", got, c.close)
			}
			if saved := f.saved(t); len(saved) != 0 {
				t.Errorf("saved %d tracks", len(saved))
			}
			if len(f.svc.claimed) != 0 {
				t.Errorf("window names still claimed: %v", f.svc.claimed)
			}
		})
	}
}

func TestCreateChecks(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(doc, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		req     Request
		engines *settings.Engines
		want    string
	}{
		{"no engine", Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "x"}, &settings.Engines{}, "Add an engine"},
		{"unknown repo", Request{Kind: track.Work, Repos: []string{"shop"}, Prompt: "x"}, nil, "no repo shop"},
		{"work alone", Request{Kind: track.Work, Prompt: "x"}, nil, "at least one repo"},
		{"no prompt", Request{Kind: track.Ask, Prompt: " \n"}, nil, "Enter a prompt"},
		{"review two repos", Request{Kind: track.Review, Repos: []string{"api", "web"}, ReviewRef: "feat/x", Prompt: "x"}, nil, "one repo"},
		{"review a GitLab link", Request{Kind: track.Review, Repos: []string{"api"}, ReviewRef: "https://gitlab.com/a/b/-/merge_requests/1", Prompt: "x"}, nil, "isn't a GitHub"},
		{"doc missing", Request{Kind: track.Doc, Document: "/no/such/file.md", Prompt: "x"}, nil, "no file or folder"},
		{"doc office file", Request{Kind: track.Doc, Document: doc, Prompt: "x"}, nil, "PowerPoint"},
		{"candor", Request{Kind: track.Doc, Document: filepath.Dir(doc), Candor: 11, Prompt: "x"}, nil, "Candor goes"},
		{"kind", Request{Kind: "fix", Prompt: "x"}, nil, "no track type"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			if c.engines != nil {
				f.engines = *c.engines
			}
			_, err := f.svc.Create(context.Background(), c.req, func(string) {})
			var p Problem
			if !errors.As(err, &p) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want a problem with %q", err, c.want)
			}
			if len(f.windows.opened) != 0 || len(f.saved(t)) != 0 {
				t.Error("a rejected request made something")
			}
		})
	}
}

func TestCreateRunsOnTheTypesAgent(t *testing.T) {
	ctx := context.Background()
	doc := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(doc, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	f.engines = settings.Engines{Claude: &settings.Engine{Model: "opus"}, Cursor: &settings.Engine{Model: "gpt-5"}}
	f.types.Set("ask", &settings.TrackType{Engine: "cursor"})
	f.types.Set("plan", &settings.TrackType{Engine: "claude", Model: "sonnet"})
	for _, c := range []struct {
		kind          track.Kind
		engine, model string
	}{
		{track.Ask, "cursor", "gpt-5"},
		{track.Plan, "claude", "sonnet"},
		{track.Doc, "claude", "opus"},
	} {
		req := Request{Kind: c.kind, Prompt: "Why"}
		if c.kind == track.Doc {
			req.Document = doc
		}
		got, err := f.svc.Create(ctx, req, func(string) {})
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if got.Track.Engine != c.engine || got.Track.Model != c.model {
			t.Errorf("%s runs on %s, %q; want %s, %q", c.kind, got.Track.Engine, got.Track.Model, c.engine, c.model)
		}
	}

	f.engines.Cursor = nil
	opened := len(f.windows.opened)
	_, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	var p Problem
	if !errors.As(err, &p) || err.Error() != "Add Cursor on the Engines tab, or pick another agent for Ask tracks in Settings → Tracks." {
		t.Errorf("an agent that isn't added: %v", err)
	}
	if len(f.windows.opened) != opened {
		t.Error("a refused create opened a window")
	}
}

func TestCreateKinds(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(doc, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	f.engines = settings.Engines{Cursor: &settings.Engine{Auto: new(bool)}}
	got, err := f.svc.Create(context.Background(), Request{
		Kind: track.Doc, Document: doc, Repos: []string{"web"}, Prompt: "Review it", Opinion: true, Terminal: true,
	}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	tr := got.Track
	if tr.Engine != "cursor" || tr.Name != "spec" || tr.Candor != track.DefaultCandor || !tr.Opinion || tr.ClaimCheck || tr.Terminal {
		t.Errorf("doc track = %+v", tr)
	}
	if tr.Repos[0].Worktree != "" || f.engine.spec.Auto || f.engine.spec.Program != "agent" {
		t.Errorf("doc repos %+v, spec %+v", tr.Repos, f.engine.spec)
	}

	got, err = f.svc.Create(context.Background(), Request{
		Kind: track.Review, Repos: []string{"api"}, ReviewRef: " https://github.com/acme/api/pull/7 ", Prompt: "Review", Candor: 1,
	}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if tr := got.Track; tr.ReviewRef != "https://github.com/acme/api/pull/7" || tr.Candor != 1 || tr.Document != "" {
		t.Errorf("review track = %+v", tr)
	}
}

func TestWindowNames(t *testing.T) {
	f := newFixture(t)
	f.windows.windows = []trackwin.Info{{Number: 1, Window: "@1", Name: "fix-login"}}
	tr := track.Track{ID: "20260928-101500-abc123", Name: "Fix login"}
	first, release, err := f.svc.claimName(tr)
	if err != nil || first != "fix-login-2" {
		t.Fatalf("first = %q, %v; want the -2 of an open window", first, err)
	}
	second, _, _ := f.svc.claimName(tr)
	if second != "fix-login-3" {
		t.Errorf("second = %q; want the -3, while -2 is being created", second)
	}
	release()
	if third, _, _ := f.svc.claimName(tr); third != "fix-login-2" {
		t.Errorf("after release = %q; want the -2 again", third)
	}
}

func TestSweepAndEnd(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var ids []string
	for _, prompt := range []string{"one", "two", "three"} {
		got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: prompt}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.Track.ID)
	}

	if err := f.svc.End(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.windows.closed, []string{"@one"}) {
		t.Errorf("End closed %v", f.windows.closed)
	}
	// The user closes two's window.
	f.windows.windows = slices.DeleteFunc(f.windows.windows, func(in trackwin.Info) bool { return in.Name == "two" })
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	listed, err := f.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 || listed[0].ID != ids[2] || listed[0].Window != "@three" {
		t.Fatalf("listed = %+v, want three open, then the ended ones", listed)
	}
	for _, l := range listed[1:] {
		if l.Open() || l.Window != "" || l.Number != 0 {
			t.Errorf("ended track listed as %+v", l)
		}
	}
	if saved := f.saved(t); len(saved) != 1 {
		t.Errorf("open in the database: %d, want 1", len(saved))
	}
}
