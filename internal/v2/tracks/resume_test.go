package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// ended creates a work track with a terminal, ends it and, when
// cleaned, cleans it.
func (f *fixture) ended(t *testing.T, cleaned bool) track.Track {
	ctx := context.Background()
	got, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api", "web"}, Prompt: "Fix it", Terminal: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, got.Track.ID); err != nil {
		t.Fatal(err)
	}
	if cleaned {
		if _, err := f.svc.Clean(ctx, got.Track.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := f.store.Track(ctx, got.Track.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestResume(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tr := f.ended(t, true)
	// Another window took the name meanwhile.
	f.windows.windows = append(f.windows.windows, trackwin.Info{Number: 1, Window: "@other", Track: "other", Name: tr.Name})

	var steps []string
	got, err := f.svc.Resume(ctx, tr.ID, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Track.Name != "fix-it-2" || got.Window.ID != "@fix-it-2" {
		t.Errorf("resumed as %q in %q, want fix-it-2", got.Track.Name, got.Window.ID)
	}
	spec := f.windows.opened[len(f.windows.opened)-1]
	if spec.Track != tr.ID || spec.Name != "fix-it-2" || spec.Terminals != 1 || spec.Agent.Title != "Claude Code" {
		t.Errorf("window = %+v", spec)
	}
	if e := f.engine.spec; !e.Resume || !e.Auto || e.Track.Session != "session-1" {
		t.Errorf("engine spec = %+v", e)
	}
	want := []string{"Re-creating the worktree for api…", "Re-creating the worktree for web…", "Starting Claude Code…"}
	if !slices.Equal(steps, want) {
		t.Errorf("progress = %q", steps)
	}
	saved, err := f.store.Track(ctx, tr.ID)
	if err != nil || !saved.Open() || saved.Cleaned() || saved.Name != "fix-it-2" {
		t.Errorf("saved = %+v, %v", saved, err)
	}
	if len(f.svc.busy) != 0 || len(f.svc.claimed) != 0 {
		t.Errorf("still held: %v, %v", f.svc.busy, f.svc.claimed)
	}
}

func TestResumeRollsBack(t *testing.T) {
	for _, c := range []struct {
		step           string
		fail           func(*fixture)
		removed, close bool
	}{
		{"worktrees", func(f *fixture) { f.worktrees.failRestore = true }, false, false},
		{"command", func(f *fixture) { f.engine.failCommand = true }, true, false},
		{"window", func(f *fixture) { f.windows.fail = true }, true, true},
		{"save", func(f *fixture) { f.store.fail = true }, true, true},
	} {
		t.Run(c.step, func(t *testing.T) {
			f := newFixture(t)
			tr := f.ended(t, true)
			f.windows.closed, f.worktrees.cleaned = nil, nil
			c.fail(f)
			if _, err := f.svc.Resume(context.Background(), tr.ID, func(string) {}); !errors.Is(err, errStep) {
				t.Fatalf("err = %v", err)
			}
			if got := slices.Equal(f.worktrees.cleaned, []string{tr.ID}); got != c.removed {
				t.Errorf("re-created worktrees removed: %v, want %v", got, c.removed)
			}
			if got := len(f.windows.closed) == 1; got != c.close {
				t.Errorf("window closed: %v, want %v", got, c.close)
			}
			if saved, _ := f.store.Track(context.Background(), tr.ID); saved.Open() || !saved.Cleaned() {
				t.Errorf("the track didn't stay ended and cleaned: %+v", saved)
			}
			if len(f.svc.busy) != 0 || len(f.svc.claimed) != 0 {
				t.Errorf("still held: %v, %v", f.svc.busy, f.svc.claimed)
			}
		})
	}
}

func TestResumeChecks(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	tr := f.ended(t, false)
	open, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, id, want string
		prepare        func()
	}{
		{"open", open.Track.ID, "already open", func() {}},
		{"gone", "20260928-101500-ffffff", "That track is gone.", func() {}},
		{"busy", tr.ID, "fix-it is busy.", func() { f.svc.busy = map[string]bool{tr.ID: true} }},
		{"engine removed", tr.ID, "Add Claude Code on the Engines tab to resume this track.", func() {
			f.svc.busy = nil
			f.engines = settings.Engines{Cursor: &settings.Engine{}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.prepare()
			opened := len(f.windows.opened)
			_, err := f.svc.Resume(ctx, c.id, func(string) {})
			var p Problem
			if !errors.As(err, &p) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want a problem with %q", err, c.want)
			}
			if len(f.windows.opened) != opened {
				t.Error("a refused resume opened a window")
			}
		})
	}
}

func TestClean(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ask, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Clean(ctx, ask.Track.ID, true); err == nil || !strings.Contains(err.Error(), "has no worktrees") {
		t.Errorf("cleaning an ask track: %v", err)
	}
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Clean(ctx, work.Track.ID, true); err == nil || !strings.Contains(err.Error(), "End fix-it before") {
		t.Errorf("cleaning an open track: %v", err)
	}

	id := work.Track.ID
	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.worktrees.unsaved = []workspace.Unsaved{{Repo: "api", Changed: 3}}
	if unsaved, err := f.svc.Unsaved(ctx, id); err != nil || len(unsaved) != 1 {
		t.Fatalf("Unsaved = %v, %v", unsaved, err)
	}
	unsaved, err := f.svc.Clean(ctx, id, false)
	if err != nil || len(unsaved) != 1 || unsaved[0].String() != "api: 3 changed files" {
		t.Fatalf("Clean = %v, %v; want the unsaved work", unsaved, err)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Cleaned() || len(f.worktrees.cleaned) != 0 {
		t.Fatal("Clean removed worktrees with unsaved work")
	}

	if unsaved, err := f.svc.Clean(ctx, id, true); err != nil || len(unsaved) != 0 {
		t.Fatalf("forced Clean = %v, %v", unsaved, err)
	}
	if saved, _ := f.store.Track(ctx, id); !saved.Cleaned() || !slices.Equal(f.worktrees.cleaned, []string{id}) {
		t.Errorf("after Clean: %+v, removed %v", saved, f.worktrees.cleaned)
	}
	if _, err := f.svc.Clean(ctx, id, true); err != nil || len(f.worktrees.cleaned) != 1 {
		t.Errorf("cleaning twice: %v, removed %v", err, f.worktrees.cleaned)
	}
}

func TestSweepHeals(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	var ids []string
	for _, prompt := range []string{"one", "two"} {
		got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: prompt}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.Track.ID)
	}
	// one was recorded as ended while its window stayed, and two's
	// window is gone while it's busy.
	if err := f.store.CloseTrack(ctx, ids[0], f.svc.now()); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = slices.DeleteFunc(f.windows.windows, func(in trackwin.Info) bool { return in.Name == "two" })
	f.windows.windows[0].Name = "one-renamed"
	f.svc.busy = map[string]bool{ids[1]: true}

	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if one, _ := f.store.Track(ctx, ids[0]); !one.Open() || one.Name != "one-renamed" {
		t.Errorf("one = %+v, want it open again under its window's name", one)
	}
	if two, _ := f.store.Track(ctx, ids[1]); !two.Open() {
		t.Error("the sweep closed a busy track")
	}
}
