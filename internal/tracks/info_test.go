package tracks

import (
	"context"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestInfo(t *testing.T) {
	for _, tc := range []struct {
		t    track.Track
		want string
	}{
		{track.Track{Name: "api-auth", Engine: "claude", Model: "opus"}, "api-auth · active · Claude Code opus"},
		{track.Track{Name: "api-auth", Title: "Fix login", Engine: "cursor", State: track.State{Waiting: true}}, "Fix login · action required · Cursor"},
	} {
		if got := Info(tc.t); got != tc.want {
			t.Errorf("Info = %q, want %q", got, tc.want)
		}
	}
}

func TestSyncInfo(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	created, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	window := created.Window.ID
	sync := func() {
		t.Helper()
		if err := f.svc.SyncInfo(ctx); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	if got, want := f.windows.info[window], created.Track.Name+" · active · Claude Code opus"; got != want {
		t.Errorf("the window shows %q, want %q", got, want)
	}
	sync()
	if f.windows.infoWrites != 1 {
		t.Errorf("%d writes for unchanged info, want 1", f.windows.infoWrites)
	}
	if err := f.svc.Report(ctx, created.Track.ID, track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	sync()
	if got, want := f.windows.info[window], created.Track.Name+" · action required · Claude Code opus"; got != want {
		t.Errorf("after a dialog opened, the window shows %q, want %q", got, want)
	}
}
