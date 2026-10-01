package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// ended creates a work track with a terminal, ends it and, when
// cleaned, archives and unarchives it, which leaves it cleaned.
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
		if _, err := f.svc.Archive(ctx, got.Track.ID, true); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Unarchive(ctx, got.Track.ID); err != nil {
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
	tr := f.ended(t, false)
	// Another window took the name meanwhile.
	f.windows.windows = append(f.windows.windows, trackwin.Info{Number: 1, Window: "@other", Track: "other", Name: tr.Name})

	var steps []string
	got, err := f.svc.Resume(ctx, tr.ID, false, func(s string) { steps = append(steps, s) })
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
	if !slices.Equal(steps, []string{"Starting Claude Code…"}) {
		t.Errorf("progress = %q", steps)
	}
	saved, err := f.store.Track(ctx, tr.ID)
	if err != nil || !saved.Open() || saved.Name != "fix-it-2" {
		t.Errorf("saved = %+v, %v", saved, err)
	}
	if len(f.svc.busy) != 0 || len(f.svc.claimed) != 0 {
		t.Errorf("still held: %v, %v", f.svc.busy, f.svc.claimed)
	}
}

func TestResumeMissingWorktree(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tr := f.ended(t, false)
	f.worktrees.gone = map[string]bool{"web": true}

	_, err := f.svc.Resume(ctx, tr.ID, false, func(string) {})
	var missing Missing
	if !errors.As(err, &missing) || len(missing) != 1 || missing[0].Name != "web" {
		t.Fatalf("err = %v, want web missing", err)
	}
	if saved, _ := f.store.Track(ctx, tr.ID); saved.Open() || len(f.windows.opened) != 1 {
		t.Fatal("Resume went on without the worktree")
	}

	var steps []string
	if _, err := f.svc.Resume(ctx, tr.ID, true, func(s string) { steps = append(steps, s) }); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Re-creating the worktree for web…", "Starting Claude Code…"}; !slices.Equal(steps, want) {
		t.Errorf("progress = %q", steps)
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
			tr := f.ended(t, false)
			f.worktrees.gone = map[string]bool{"api": true, "web": true}
			f.windows.closed = nil
			c.fail(f)
			if _, err := f.svc.Resume(context.Background(), tr.ID, true, func(string) {}); !errors.Is(err, errStep) {
				t.Fatalf("err = %v", err)
			}
			if got := slices.Equal(f.worktrees.cleaned, []string{tr.ID}); got != c.removed {
				t.Errorf("re-created worktrees removed: %v, want %v", got, c.removed)
			}
			if got := len(f.windows.closed) == 1; got != c.close {
				t.Errorf("window closed: %v, want %v", got, c.close)
			}
			if saved, _ := f.store.Track(context.Background(), tr.ID); saved.Open() {
				t.Errorf("the track didn't stay ended: %+v", saved)
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
			_, err := f.svc.Resume(ctx, c.id, true, func(string) {})
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

func TestResumeCleaned(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	tr := f.ended(t, true)
	f.worktrees.gone = map[string]bool{"api": true, "web": true}
	var missing Missing
	if _, err := f.svc.Resume(ctx, tr.ID, false, func(string) {}); !errors.As(err, &missing) || len(missing) != 2 {
		t.Fatalf("resuming without re-creating: %v, want both worktrees missing", err)
	}
	if _, err := f.svc.Resume(ctx, tr.ID, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if saved, _ := f.store.Track(ctx, tr.ID); !saved.Open() || saved.Cleaned() {
		t.Errorf("resumed = %+v, want open and no longer cleaned", saved.State)
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
	if err := f.store.SetState(ctx, ids[0], track.State{ClosedAt: f.svc.now()}); err != nil {
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
