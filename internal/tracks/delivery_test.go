package tracks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
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
	for _, e := range []track.Event{track.Resumed, track.Ended, track.Interrupted, track.AgentExited, track.AgentFailed} {
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
		f.windows.scrolled = nil
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

// deliveryFixture is track a, open in window @a with its agent idle,
// and a clock the test moves.
func deliveryFixture(t *testing.T) (*fixture, *time.Time) {
	t.Helper()
	f := trackA(t)
	now := time.Unix(1_800_000_000, 0)
	f.svc.Now = func() time.Time { return now }
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@a", Track: "a", Name: "a"})
	f.svc.SetTurn(context.Background(), "a", hooks.TurnEnd)
	return f, &now
}

func post(t *testing.T, f *fixture, m relay.Message) relay.Message {
	t.Helper()
	m, err := f.store.AddRelayMessage(context.Background(), "a", m)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func deliver(t *testing.T, f *fixture) []string {
	t.Helper()
	before := len(f.windows.pasted)
	if err := f.svc.Deliver(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f.windows.pasted[before:]
}

func TestDeliverPointsOnceNothingHoldsItBack(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: relay.TracksHandle, Kind: relay.Notice, Body: "@reviewer-1 joined", At: *now})
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a message younger than the grace was pointed to: %v", got)
	}
	*now = now.Add(pointerGrace)
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("an agent in a turn was pointed to: %v", got)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	got := deliver(t, f)
	if len(got) != 1 || got[0] != "@a 1 message from @reviewer-1 on the relay: run `tracks relay read`." {
		t.Fatalf("pasted %v, want one pointer to the report, not the notice", got)
	}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a pointer waiting for its turn went again: %v", got)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	*now = now.Add(time.Minute)
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a message pointed to once was pointed to again: %v", got)
	}

	answer := post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Answer, Body: "Because.", At: *now})
	*now = now.Add(pointerGrace)
	if _, err := f.store.MarkRelayRead(ctx, "a", relay.TrackHandle, answer.ID); err != nil {
		t.Fatal(err)
	}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a message read already was pointed to: %v", got)
	}
}

func TestDeliverWaitsForWhatHoldsItBack(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("pointed while the user typed: %v", got)
	}
	f.windows.lastKeys = nil
	f.windows.screens = map[string]string{"@a": "──────────────────\n\x1b[39m❯ half a thought\n──────────────────"}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("pointed over a draft: %v", got)
	}
	f.windows.screens = nil
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v once nothing held it back, want the pointer", got)
	}
}

func TestDeliverTriesAgainWhenTheAgentDidntTakeIt(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v, want the pointer", got)
	}
	*now = now.Add(pointerStall)
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a stalled pointer went again at once: %v", got)
	}
	*now = now.Add(pointerRetry)
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v after the retry's wait, want the pointer again", got)
	}
}

func TestANewAgentIsPointedAgain(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	deliver(t, f)
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	deliver(t, f)
	if err := f.svc.Report(ctx, "a", track.Resumed); err != nil {
		t.Fatal(err)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 1 {
		t.Errorf("a new agent wasn't pointed to the unread report: %v", got)
	}
}

func TestAShortTurnTookThePointer(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	deliver(t, f)
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	*now = now.Add(pointerStall)
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a turn that came and went between ticks counted as a stall: %v", got)
	}
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Answer, Body: "Later.", At: *now})
	*now = now.Add(pointerGrace)
	got := deliver(t, f)
	if len(got) != 1 || !strings.HasPrefix(got[0], "@a 1 message") {
		t.Errorf("pasted %v, want a pointer to the new message alone, at once", got)
	}
}

func TestNoPointerWhereTheAgentIsGone(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.windows[0].Exit = "0"
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a pointer went into the shell an exited agent left: %v", got)
	}
	f.windows.windows[0].Exit = ""
	_, release, err := f.svc.hold(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if got := deliver(t, f); len(got) != 0 {
		t.Errorf("a pointer went into a track being ended or promoted: %v", got)
	}
	release()
}

func TestATurnDuringThePasteHolds(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.whilePasting = func() { f.svc.SetTurn(ctx, "a", hooks.TurnStart) }
	deliver(t, f)
	f.windows.whilePasting = nil
	if !f.svc.inTurn("a") {
		t.Fatal("the paste's end undid the turn that started during it")
	}
	*now = now.Add(pointerStall)
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 0 {
		t.Errorf("the pointer taken during its paste went again: %v", got)
	}
}

func TestAFailedPasteIsTriedAgain(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.pasteErr = errors.New("tmux is gone")
	if err := f.svc.Deliver(context.Background()); err == nil {
		t.Fatal("a failed paste said nothing")
	}
	f.windows.pasteErr = nil
	if got := deliver(t, f); len(got) != 1 {
		t.Errorf("pasted %v after a failed paste, want the pointer again", got)
	}
}

func TestATurnDuringTheChecksHolds(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.whileChecking = func() { f.svc.SetTurn(ctx, "a", hooks.TurnStart) }
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a pointer went into a turn that started while tmux was asked: %v", got)
	}
	f.windows.whileChecking = nil
	if !f.svc.inTurn("a") {
		t.Error("the turn that started during the checks was lost")
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 1 {
		t.Errorf("pasted %v once the turn ended, want the pointer", got)
	}
}

func TestPromoteForgetsTheTurn(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Plan, Repos: []string{"api"}, Prompt: "Plan the cache"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	f.engine.replies = map[string]string{made.Track.Session: "1. Warm the cache"}
	id := made.Track.ID
	f.svc.SetTurn(ctx, id, hooks.TurnEnd)
	if f.svc.inTurn(id) {
		t.Fatal("the plan's agent should be idle")
	}
	if _, err := f.svc.Promote(ctx, id, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if !f.svc.inTurn(id) {
		t.Error("the old agent's ended turn holds for the promoted one")
	}
}

func TestNoPointerWhileTheUserScrollsBack(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.scrolled = map[string]bool{"@a": true}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("a pointer went into a pane in copy mode: %v", got)
	}
	f.windows.scrolled = nil
	if got := deliver(t, f); len(got) != 1 {
		t.Errorf("pasted %v once the user left the scrollback, want the pointer", got)
	}
}

func TestAHeldPointerIsShownAndCollected(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("pointed while the user typed: %v", got)
	}
	if !f.svc.heldFrom("a", "reviewer-1") {
		t.Fatal("a pointer held back by typing isn't shown as held")
	}
	f.windows.scrolled = map[string]bool{"@a": true}
	name, err := f.svc.CollectMessages(ctx, "a")
	if err != nil || name != "a" {
		t.Fatalf("collect: %q, %v", name, err)
	}
	if len(f.windows.pasted) != 1 || f.windows.scrolled["@a"] {
		t.Fatalf("pasted %v, scrolled %v; want the pointer, out of the scrollback", f.windows.pasted, f.windows.scrolled)
	}
	if f.svc.heldFrom("a", "reviewer-1") {
		t.Error("a collected pointer is still shown as held")
	}
	var problem Problem
	if _, err := f.svc.CollectMessages(ctx, "a"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "No messages") {
		t.Errorf("collecting what was collected: %v, want No messages", err)
	}
	// A delivered pointer, waiting for its turn, is on its way.
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Answer, Body: "Later.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = nil
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v, want the pointer delivered", got)
	}
	if _, err := f.svc.CollectMessages(ctx, "a"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "on its way") {
		t.Errorf("collecting with a pointer on its way: %v", err)
	}
}

func TestHeldGoesWhenTheAgentExited(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	deliver(t, f)
	f.windows.windows[0].Exit = "0"
	deliver(t, f)
	if f.svc.heldFrom("a", "reviewer-1") {
		t.Error("Collect messages stayed for an agent that exited in tmux")
	}
}

func TestAFailedCollectKeepsItHeld(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	deliver(t, f)
	// In a turn, which a turn's start during the paste mustn't take as
	// the pointer taken, nor a failed collect undo.
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	f.windows.pasteErr = errors.New("tmux is gone")
	f.windows.whilePasting = func() {
		f.svc.SetTurn(ctx, "a", hooks.TurnStart)
		if !f.svc.heldFrom("a", "reviewer-1") {
			t.Error("Collect messages went before the paste was in")
		}
	}
	if _, err := f.svc.CollectMessages(ctx, "a"); err == nil {
		t.Fatal("a failed paste said nothing")
	}
	if !f.svc.heldFrom("a", "reviewer-1") {
		t.Error("a failed collect hid Collect messages")
	}
	if !f.svc.inTurn("a") {
		t.Error("a failed collect marked the agent, in a turn, idle")
	}
	f.windows.pasteErr, f.windows.whilePasting = nil, nil
	f.windows.scrollErr = errors.New("tmux can't tell")
	if _, err := f.svc.CollectMessages(ctx, "a"); !errors.Is(err, f.windows.scrollErr) || len(f.windows.pasted) != 1 {
		t.Errorf("collect pasted %v into a pane whose mode it couldn't tell: %v", f.windows.pasted, err)
	}
	// The failed collect, and the turn's start during it, left the
	// messages to collect.
	f.windows.scrollErr = nil
	if _, err := f.svc.CollectMessages(ctx, "a"); err != nil || len(f.windows.pasted) != 2 {
		t.Errorf("collect after the failures: %v, pasted %v", err, f.windows.pasted)
	}
}

func TestCollectRefuses(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	var problem Problem
	if _, err := f.svc.CollectMessages(ctx, "a"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "No messages") {
		t.Errorf("nothing to collect: %v", err)
	}
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	if err := f.svc.Report(ctx, "a", track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CollectMessages(ctx, "a"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "answer it first") {
		t.Errorf("a question on screen: %v", err)
	}
	if err := f.svc.Report(ctx, "a", track.AgentWorking); err != nil {
		t.Fatal(err)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	if _, err := f.svc.CollectMessages(ctx, "a"); err != nil || len(f.windows.pasted) != 1 {
		t.Errorf("the user collected in a turn: %v, pasted %v; want the pointer, as they asked", err, f.windows.pasted)
	}
}

func TestACollectedPointerIsntRetried(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	if _, err := f.svc.CollectMessages(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(pointerStall + pointerRetry + pointerGrace)
	if got := deliver(t, f); len(got) != 0 {
		t.Errorf("a collected pointer went again after the stall's wait: %v", got)
	}
	if !f.svc.inTurn("a") {
		t.Error("collecting in a turn marked the agent idle")
	}
}

func TestHeldGoesWithTheAgent(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	deliver(t, f)
	if !f.svc.heldFrom("a", "reviewer-1") {
		t.Fatal("not held")
	}
	if err := f.svc.Report(ctx, "a", track.AgentExited); err != nil {
		t.Fatal(err)
	}
	if f.svc.heldFrom("a", "reviewer-1") {
		t.Error("Collect messages stayed after the agent exited")
	}
}

func TestADeliveredPointerIsHeldNoMore(t *testing.T) {
	f, now := deliveryFixture(t)
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	f.windows.lastKeys = map[string]time.Time{"@a": now.Add(-time.Second)}
	deliver(t, f)
	if !f.svc.heldFrom("a", "reviewer-1") {
		t.Fatal("not held while the user typed")
	}
	f.windows.lastKeys = nil
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v once the typing stopped", got)
	}
	if f.svc.heldFrom("a", "reviewer-1") {
		t.Error("Collect messages stayed while the delivered pointer waits for its turn")
	}
}
