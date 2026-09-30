package tracks

import (
	"context"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestRestart(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id, window := made.Track.ID, made.Window.ID
	if _, err := f.svc.Restart(ctx, id, func(string) {}); err == nil || !strings.HasSuffix(err.Error(), "'s agent is still running.") {
		t.Errorf("restarting a running agent: %v", err)
	}

	f.windows.windows[0].Exit = "1"
	if _, err := f.svc.CheckExits(ctx); err != nil {
		t.Fatal(err)
	}
	var steps []string
	got, err := f.svc.Restart(ctx, id, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Window.ID != window || len(f.windows.respawned) != 1 || f.windows.windows[0].Exit != "" {
		t.Errorf("restarted in %+v, respawned %+v, exit %q; want the same window's agent restarted", got.Window, f.windows.respawned, f.windows.windows[0].Exit)
	}
	if !f.engine.spec.Resume || len(f.engine.spec.DraftPRs) != 0 || len(steps) == 0 {
		t.Errorf("spec %+v, steps %v; want the session resumed", f.engine.spec, steps)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Status() != track.Active || f.windows.attention[window] {
		t.Errorf("saved %s, attention %v; want active again", saved.Status().ID, f.windows.attention[window])
	}

	f.engine.unstarted = true
	f.windows.windows[0].Exit = "127"
	if _, err := f.svc.Restart(ctx, id, func(string) {}); err != nil {
		t.Fatalf("restarting before the exit was recorded: %v", err)
	}
	if f.engine.spec.Resume || len(f.engine.spec.DraftPRs) != 1 {
		t.Errorf("spec %+v; a session that never started starts fresh, on the prompt", f.engine.spec)
	}
}

func TestRestartRefuses(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why?"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	f.windows.windows[0].Exit = "0"
	f.engines = settings.Engines{Cursor: &settings.Engine{}}
	if _, err := f.svc.Restart(ctx, made.Track.ID, func(string) {}); err == nil || err.Error() != "Add Claude Code on the Engines tab to restart this track." {
		t.Errorf("an engine that isn't added: %v", err)
	}
	if err := f.svc.End(ctx, made.Track.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Restart(ctx, made.Track.ID, func(string) {}); err == nil || !strings.HasSuffix(err.Error(), " has ended: resume it instead.") {
		t.Errorf("an ended track: %v", err)
	}
	if len(f.windows.respawned) != 0 {
		t.Errorf("respawned %+v", f.windows.respawned)
	}
}
