package tracks

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

func TestInterruptAndReopen(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var ids []string
	for _, req := range []Request{
		{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"},
		{Kind: track.Ask, Repos: []string{"web"}, Prompt: "Why"},
		{Kind: track.Ask, Repos: []string{"web"}, Prompt: "Still here"},
	} {
		c, err := f.svc.Create(ctx, req, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.Track.ID)
	}
	if err := f.svc.Report(ctx, ids[1], track.AgentExited); err != nil {
		t.Fatal(err)
	}
	// The tmux server went away with the first two windows; the third
	// stays, as if its window outlived a daemon restart.
	f.windows.windows = slices.DeleteFunc(f.windows.windows, func(in trackwin.Info) bool { return in.Track != ids[2] })

	names, err := f.svc.Interrupt(ctx)
	if err != nil || !slices.Equal(names, []string{"fix-it", "why"}) {
		t.Fatalf("Interrupt = %q, %v; want fix-it and why", names, err)
	}
	for i, want := range []bool{true, true, false} {
		got, _ := f.store.Track(ctx, ids[i])
		if got.Interrupted != want || got.Open() == want {
			t.Errorf("track %d = %+v, want interrupted %v", i, got.State, want)
		}
	}
	if again, err := f.svc.Interrupt(ctx); err != nil || len(again) != 0 {
		t.Errorf("Interrupt again = %q, %v; want nothing", again, err)
	}

	f.worktrees.gone = map[string]bool{"api": true}
	var steps []string
	got, err := f.svc.Reopen(ctx, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids[0] || !strings.Contains(got[0].Error, "api") || got[0].Window != "" {
		t.Errorf("Reopen = %+v; want fix-it failing on its worktree", got)
	}
	if len(got) == 2 && (got[1].ID != ids[1] || got[1].Error != "" || got[1].Window != "@why") {
		t.Errorf("Reopen = %+v; want why in @why", got)
	}
	if want := []string{"Reopening fix-it…", "Reopening why…", "Starting Claude Code…"}; !slices.Equal(steps, want) {
		t.Errorf("progress = %q, want %q", steps, want)
	}
	if fix, _ := f.store.Track(ctx, ids[0]); fix.Open() || !fix.Interrupted {
		t.Errorf("fix-it = %+v, want still ended and interrupted", fix.State)
	}
	if why, _ := f.store.Track(ctx, ids[1]); !why.Open() || why.Interrupted || why.Exit != "" {
		t.Errorf("why = %+v, want open again with its agent running", why.State)
	}
	if left, _ := f.svc.Interrupted(ctx); len(left) != 1 || left[0].ID != ids[0] {
		t.Errorf("Interrupted = %+v, want fix-it alone", left)
	}
}
