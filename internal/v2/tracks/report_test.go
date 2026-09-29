package tracks

import (
	"context"
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestReport(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "one"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := got.Track.ID
	status := func() track.Status {
		t.Helper()
		tr, err := f.store.Track(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.Status()
	}
	if s := status(); s != track.Active {
		t.Fatalf("a new track is %s", s.ID)
	}
	steps := []struct {
		e    track.Event
		want track.Status
	}{
		{track.AgentWaiting, track.ActionRequired},
		{track.AgentWorking, track.Active},
		{track.AgentWaiting, track.ActionRequired},
		{track.Ended, track.Done},
		{track.AgentWaiting, track.Done},
	}
	window := got.Window.ID
	for _, st := range steps {
		if err := f.svc.Report(ctx, id, st.e); err != nil {
			t.Fatal(err)
		}
		if s := status(); s != st.want {
			t.Errorf("after %s the track is %s, want %s", st.e, s.ID, st.want.ID)
		}
		if st.want != track.Done && f.windows.attention[window] != st.want.Attention {
			t.Errorf("after %s the window's mark is %v", st.e, f.windows.attention[window])
		}
	}

	var p Problem
	if err := f.svc.Report(ctx, id, "agent.dancing"); !errors.As(err, &p) {
		t.Errorf("an unknown event: %v, want a Problem", err)
	}
	if err := f.svc.Report(ctx, "nope", track.Ended); !errors.As(err, &p) {
		t.Errorf("an unknown track: %v, want a Problem", err)
	}
}
