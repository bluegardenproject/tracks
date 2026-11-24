package tracks

import (
	"context"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// trackA is a fixture with an open track a, whose agent runs.
func trackA(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	if err := f.store.AddTrack(context.Background(), track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTurns(t *testing.T) {
	f := trackA(t)
	if !f.svc.inTurn("a") {
		t.Error("a track Tracks hasn't heard of may be in a turn")
	}
	f.svc.SetTurn(context.Background(), "a", hooks.TurnEnd)
	if f.svc.inTurn("a") {
		t.Error("a turn that ended")
	}
	f.svc.SetTurn(context.Background(), "a", "")
	if f.svc.inTurn("a") {
		t.Error("a report without a turn changed it")
	}
	f.svc.SetTurn(context.Background(), "a", hooks.TurnStart)
	if !f.svc.inTurn("a") {
		t.Error("a turn that started")
	}
}

func TestANewAgentForgetsTheTurn(t *testing.T) {
	ctx := context.Background()
	for _, e := range []track.Event{track.Resumed, track.Ended, track.Interrupted, track.AgentExited} {
		f := trackA(t)
		f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
		if err := f.svc.Report(ctx, "a", e); err != nil {
			t.Fatal(err)
		}
		if !f.svc.inTurn("a") {
			t.Errorf("after %s the old agent's ended turn still holds", e)
		}
	}
}

func TestBlockers(t *testing.T) {
	f := trackA(t)
	now := time.Unix(1_800_000_000, 0)
	f.svc.Now = func() time.Time { return now }
	empty := "────────────────────\n\x1b[39m❯ \x1b[2mTry it\x1b[0m\n────────────────────"
	draft := "────────────────────\n\x1b[39m❯ half a thought\n────────────────────"
	tr := track.Track{ID: "a", Engine: "claude"}
	for _, tc := range []struct {
		name    string
		turn    string
		waiting bool
		key     time.Time
		screen  string
		want    relay.Blockers
	}{
		{"free", hooks.TurnEnd, false, now.Add(-time.Minute), empty, relay.Blockers{}},
		{"in a turn", hooks.TurnStart, false, time.Time{}, empty, relay.Blockers{InTurn: true}},
		{"asking", hooks.TurnEnd, true, time.Time{}, empty, relay.Blockers{Asking: true}},
		{"typing", hooks.TurnEnd, false, now.Add(-time.Second), empty, relay.Blockers{Typing: true}},
		{"a draft", hooks.TurnEnd, false, time.Time{}, draft, relay.Blockers{Draft: true}},
		{"a screen it can't read", hooks.TurnEnd, false, time.Time{}, "a dialog", relay.Blockers{}},
	} {
		f.svc.SetTurn(context.Background(), "a", tc.turn)
		f.windows.lastKeys = map[string]time.Time{"@a": tc.key}
		f.windows.screens = map[string]string{"@a": tc.screen}
		tr.Waiting = tc.waiting
		if got := f.svc.blockers(tr, "@a"); got != tc.want {
			t.Errorf("%s: %+v, want %+v (%s)", tc.name, got, tc.want, got.Why())
		}
	}
}

func TestNoTurnWithoutAnAgent(t *testing.T) {
	ctx := context.Background()
	f := trackA(t)
	if err := f.svc.Report(ctx, "a", track.AgentExited); err != nil {
		t.Fatal(err)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if !f.svc.inTurn("a") {
		t.Error("a late Stop of an agent that exited noted a turn")
	}
	f.svc.SetTurn(ctx, "gone", hooks.TurnEnd)
	if !f.svc.inTurn("gone") {
		t.Error("a track that isn't there noted a turn")
	}
}
