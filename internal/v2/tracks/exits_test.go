package tracks

import (
	"context"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestCheckExits(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	var ids []string
	for _, prompt := range []string{"one", "two", "three"} {
		got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: prompt}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.Track.ID)
	}
	if err := f.svc.Report(ctx, ids[0], track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	f.windows.windows[0].Exit = "1"
	f.windows.windows[1].Exit = "0"
	check := func() []Exit {
		t.Helper()
		exits, err := f.svc.CheckExits(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return exits
	}
	status := func(id string) track.Status {
		t.Helper()
		tr, err := f.store.Track(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.Status()
	}

	names := []string{f.windows.windows[0].Name, f.windows.windows[1].Name}
	if got := check(); !slices.Equal(got, []Exit{{names[0], "1"}, {names[1], "0"}}) {
		t.Errorf("exits = %v", got)
	}
	if status(ids[0]) != track.Error || status(ids[1]) != track.Exited || status(ids[2]) != track.Active {
		t.Errorf("statuses %s, %s, %s; want error, agent exited, active", status(ids[0]).ID, status(ids[1]).ID, status(ids[2]).ID)
	}
	if !f.windows.attention[f.windows.windows[0].Window] || f.windows.attention[f.windows.windows[1].Window] {
		t.Errorf("attention %v; only the failed agent's window needs the user", f.windows.attention)
	}
	if got := check(); len(got) != 0 {
		t.Errorf("reported again: %v", got)
	}

	f.windows.windows[1].Exit = "130"
	if got := check(); !slices.Equal(got, []Exit{{names[1], "130"}}) || status(ids[1]) != track.Error {
		t.Errorf("an exit code that changed: %v, %s", got, status(ids[1]).ID)
	}

	ended := f.windows.windows[1]
	if err := f.svc.End(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	ended.Exit = "1"
	f.windows.windows = append(f.windows.windows, ended)
	if got := check(); len(got) != 0 {
		t.Errorf("an ended track reported: %v", got)
	}
}
