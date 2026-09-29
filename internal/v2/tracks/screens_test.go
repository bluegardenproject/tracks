package tracks

import (
	"context"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestCheckScreens(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id, window := got.Track.ID, got.Window.ID
	status := func() track.Status {
		t.Helper()
		tr, err := f.store.Track(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.Status()
	}
	check := func() {
		t.Helper()
		if err := f.svc.CheckScreens(ctx); err != nil {
			t.Fatal(err)
		}
	}

	f.windows.screens = map[string]string{window: " Do you want to proceed?\n ❯ 1. Yes\n   2. No"}
	check()
	if status() != track.Active {
		t.Fatal("the screen alone doesn't make a Claude track wait: its hooks do")
	}
	if err := f.svc.Report(ctx, id, track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	check()
	if status() != track.ActionRequired {
		t.Fatal("an open dialog keeps the track waiting")
	}

	f.windows.screens[window] = "⏺ Interrupted by user\n> "
	check()
	if status() != track.ActionRequired {
		t.Error("one check without the dialog is too soon: it may still be drawing")
	}
	f.windows.screens[window] = " ❯ 1. Yes"
	check()
	f.windows.screens[window] = "> "
	check()
	if status() != track.ActionRequired {
		t.Error("the checks without the dialog must be in a row")
	}
	check()
	if status() != track.Active || f.windows.attention[window] {
		t.Errorf("after two checks without the dialog the track is %s, marked %v", status().ID, f.windows.attention[window])
	}
}
