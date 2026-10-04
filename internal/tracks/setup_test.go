package tracks

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// fakeSetups keeps setup panes per window; a test sets their state.
type fakeSetups struct {
	mu     sync.Mutex
	panes  map[string][]tmux.Pane
	added  []trackwin.Process
	dirs   []string
	closed []string
}

func (f *fakeSetups) Panes(window string) ([]tmux.Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.panes[window]), nil
}

func (f *fakeSetups) AddSetup(window, dir string, p trackwin.Process) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.panes == nil {
		f.panes = map[string][]tmux.Pane{}
	}
	f.added, f.dirs = append(f.added, p), append(f.dirs, dir)
	id := "%" + string(rune('a'+len(f.added)))
	f.panes[window] = append(f.panes[window], tmux.Pane{ID: id, Role: trackwin.RoleSetup, Title: p.Title, State: SetupRunning})
	return nil
}

func (f *fakeSetups) ClosePane(pane string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, pane)
	for w, panes := range f.panes {
		f.panes[w] = slices.DeleteFunc(panes, func(p tmux.Pane) bool { return p.ID == pane })
	}
	return nil
}

// setState sets the state of every setup pane titled for repo.
func (f *fakeSetups) setState(repo, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, panes := range f.panes {
		for i := range panes {
			if panes[i].Title == "setup · "+repo {
				panes[i].State = state
			}
		}
	}
}

// withSetup gives the fixture's repo name a setup command and, when
// servers, a dev server; it returns fake setup panes and the .env
// copies made.
func withSetup(t *testing.T, f *fixture, name, setup string, servers bool) (*fakeSetups, *[]string) {
	t.Helper()
	ctx := context.Background()
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		if r.Name != name {
			continue
		}
		r.Setup = setup
		if servers {
			r.Servers = []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: store.PortAssigned}}
		}
		if _, err := f.store.UpdateRepo(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	setups := &fakeSetups{}
	var copies []string
	f.svc.Setups = setups
	f.svc.CopyEnv = func(_ context.Context, primary, worktree string) ([]string, error) {
		copies = append(copies, worktree)
		return nil, nil
	}
	return setups, &copies
}

func TestWorkTracksStartTheirSetup(t *testing.T) {
	f := newFixture(t)
	setups, copies := withSetup(t, f, "api", "pnpm install", false)
	got, err := f.svc.Create(context.Background(), Request{Kind: track.Work, Repos: []string{"api", "web"}, Prompt: "go"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(setups.added) != 1 || setups.added[0].Title != "setup · api" || setups.dirs[0] != got.Track.Repos[0].Worktree {
		t.Fatalf("setup panes %+v in %v; want one for api in its worktree", setups.added, setups.dirs)
	}
	if !slices.Equal(*copies, []string{got.Track.Repos[0].Worktree}) {
		t.Errorf(".env copied into %v; want only api's worktree, the repo with a setup", *copies)
	}

	ctx := context.Background()
	states, err := f.svc.StartSetup(ctx, got.Track.ID, false)
	if err != nil || len(setups.added) != 1 || !slices.Equal(states, []SetupRepo{{Repo: "api", State: SetupRunning}}) {
		t.Fatalf("starting again while running: %+v, %v, %d panes; want no second pane", states, err, len(setups.added))
	}
	setups.setState("api", "failed 2")
	if states, _ = f.svc.StartSetup(ctx, got.Track.ID, false); len(setups.added) != 1 || states[0] != (SetupRepo{Repo: "api", State: SetupFailed, Code: 2}) {
		t.Fatalf("a failed setup without retry: %+v; want it left failed", states)
	}
	if states, _ = f.svc.StartSetup(ctx, got.Track.ID, true); len(setups.added) != 2 || len(setups.closed) != 1 || states[0].State != SetupRunning {
		t.Fatalf("retrying: %+v, %d added, %d closed; want the failed pane replaced", states, len(setups.added), len(setups.closed))
	}

	setups.panes = nil // the pane closes on success
	if err := f.svc.FinishSetup(ctx, got.Track.ID, "api"); err != nil {
		t.Fatal(err)
	}
	if states, _ = f.svc.StartSetup(ctx, got.Track.ID, true); len(setups.added) != 2 || states[0].State != SetupDone {
		t.Errorf("after it finished: %+v; want done and no new pane", states)
	}
}

func TestReviewTracksWaitUntilSetupIsNeeded(t *testing.T) {
	f := newFixture(t)
	setups, copies := withSetup(t, f, "api", "", true)
	got, err := f.svc.Create(context.Background(), Request{Kind: track.Review, Repos: []string{"api"}, ReviewRef: "feat/x", Prompt: "review"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(setups.added) != 0 || len(*copies) != 1 {
		t.Fatalf("%d setup panes, %d .env copies; want none and one, for the repo with dev servers", len(setups.added), len(*copies))
	}
	states, err := f.svc.SetupStates(context.Background(), got.Track.ID)
	if err != nil || len(states) != 0 {
		t.Errorf("a repo without a setup: %+v, %v; want no setups", states, err)
	}
}

func TestWaitSetupStartsAndWaits(t *testing.T) {
	f := newFixture(t)
	setups, _ := withSetup(t, f, "api", "pnpm install", false)
	ctx := context.Background()
	got, err := f.svc.Create(ctx, Request{Kind: track.Review, Repos: []string{"api"}, ReviewRef: "feat/x", Prompt: "review"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(2 * setupPoll)
		_ = f.svc.FinishSetup(ctx, got.Track.ID, "api")
		setups.mu.Lock()
		setups.panes = nil
		setups.mu.Unlock()
	}()
	states, err := f.svc.WaitSetup(ctx, got.Track.ID)
	if err != nil || len(setups.added) != 1 || !slices.Equal(states, []SetupRepo{{Repo: "api", State: SetupDone}}) {
		t.Errorf("WaitSetup = %+v, %v with %d panes; want it started once and done", states, err, len(setups.added))
	}
}

func TestSetupStates(t *testing.T) {
	tr := track.Track{Repos: []track.Repo{
		{Name: "api", Worktree: "/wt/api"},
		{Name: "web", Worktree: "/wt/web", SetupDone: true},
		{Name: "docs", Worktree: "/wt/docs"},
		{Name: "app", Worktree: "/wt/app", SetupDone: true},
		{Name: "new", Worktree: "/wt/new"},
	}}
	commands := map[string]string{"api": "a", "web": "b", "app": "c", "new": "d"}
	panes := []tmux.Pane{
		{Role: trackwin.RoleSetup, Title: "setup · api", State: "failed 127"},
		{Role: trackwin.RoleTerminal, Title: "setup · web", State: SetupRunning},
		{Role: trackwin.RoleSetup, Title: "setup · app", State: SetupRunning},
		{Role: trackwin.RoleSetup, Title: "setup · new"},
	}
	want := []SetupRepo{{"api", SetupFailed, 127}, {"web", SetupDone, 0}, {"app", SetupRunning, 0}, {"new", SetupRunning, 0}}
	if got := setupStates(tr, commands, panes); !slices.Equal(got, want) {
		t.Errorf("setupStates = %+v, want %+v", got, want)
	}
}

func TestResumeRedoesTheSetupOfRecreatedWorktrees(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tr := f.ended(t, false)
	withSetup(t, f, "api", "pnpm install", false)
	setups, copies := withSetup(t, f, "web", "pnpm install", false)
	for _, r := range tr.Repos {
		if err := f.store.SetSetupDone(ctx, tr.ID, r.Name, true); err != nil {
			t.Fatal(err)
		}
	}
	f.worktrees.gone = map[string]bool{"web": true}
	if _, err := f.svc.Resume(ctx, tr.ID, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	saved, err := f.store.Track(ctx, tr.ID)
	if err != nil || !saved.Repos[0].SetupDone || saved.Repos[1].SetupDone {
		t.Fatalf("saved repos %+v, %v; want api still done and web not", saved.Repos, err)
	}
	if len(setups.added) != 1 || setups.added[0].Title != "setup · web" {
		t.Errorf("setup panes %+v; want only web's started again", setups.added)
	}
	if !slices.Equal(*copies, []string{saved.Repos[1].Worktree}) {
		t.Errorf(".env copied into %v; want only the re-created worktree", *copies)
	}
}
