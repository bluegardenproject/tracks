package tracks

import (
	"context"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/train"
)

// relayOf is every message on track r's relay, oldest first, as the
// track would read it if it had read none.
func relayOf(t *testing.T, f *fixture) []relay.Message {
	t.Helper()
	var out []relay.Message
	for id := int64(1); ; id++ {
		m, err := f.store.RelayMessage(context.Background(), "r", id)
		if err != nil {
			return out
		}
		out = append(out, m)
	}
}

func TestTheReviewerOnTheRelay(t *testing.T) {
	ctx := context.Background()
	f, panes, worktree := trainsFixture(t, "cursor")
	run := startReview(t, f, worktree)
	handle := oneTrain(t, f).Handle
	if !strings.HasPrefix(handle, "reviewer-") || train.CheckHandle(handle) != nil {
		t.Fatalf("the reviewer's handle %q", handle)
	}
	msgs := relayOf(t, f)
	if len(msgs) != 1 || msgs[0].Kind != relay.Notice || msgs[0].From != relay.TracksHandle || msgs[0].To != "" ||
		msgs[0].Body != "@"+handle+" joined: Reviewer on Cursor (m1), reviewing the change." {
		t.Fatalf("after the start: %+v, want the join notice", msgs)
	}

	if err := f.svc.ReviewReply(ctx, "r", run, "Fine.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	msgs = relayOf(t, f)
	if len(msgs) != 2 || msgs[1].From != handle || msgs[1].To != relay.TrackHandle || msgs[1].Kind != relay.Report || msgs[1].Body != "Fine.\nREVIEW OUTCOME: pass" {
		t.Fatalf("after the report: %+v", msgs)
	}
	if report, err := f.svc.WaitReview(ctx, "r"); err != nil || !strings.Contains(report, "Fine.") {
		t.Fatalf("WaitReview = %q, %v", report, err)
	}
	if unread, err := f.svc.RelayRead(ctx, "r", relay.TrackHandle); err != nil || len(unread) != 0 {
		t.Errorf("tracks review printed the report, yet the relay has %+v unread, %v", unread, err)
	}

	asked, err := f.svc.RelaySend(ctx, "r", "reviewer", relay.Question, "Why the cache?")
	if err != nil {
		t.Fatal(err)
	}
	if asked.From != relay.TrackHandle || asked.To != handle || asked.Kind != relay.Question || asked.Body != "Why the cache?" {
		t.Errorf("the question as posted: %+v", asked)
	}
	if len(panes.respawned) != 1 || !strings.Contains(panes.respawned[0], questionLead+"Why the cache?") {
		t.Errorf("the question didn't reach the Cursor reviewer: %v", panes.respawned)
	}
	if err := f.svc.ReviewReply(ctx, "r", run, "Because it's hot."); err != nil {
		t.Fatal(err)
	}
	if answer, err := f.svc.WaitReview(ctx, "r"); err != nil || answer != "Because it's hot." {
		t.Fatalf("the answer: %q, %v", answer, err)
	}
	msgs = relayOf(t, f)
	if last := msgs[len(msgs)-1]; last.From != handle || last.Kind != relay.Answer || last.Body != "Because it's hot." {
		t.Errorf("the answer as posted: %+v", last)
	}
	if unread, err := f.svc.RelayRead(ctx, "r", relay.TrackHandle); err != nil || len(unread) != 0 {
		t.Errorf("the printed answer is still unread: %+v, %v", unread, err)
	}

	if err := f.svc.AskReview(ctx, "r", "And the lock?"); err != nil {
		t.Fatal(err)
	}
	if last := relayOf(t, f)[len(relayOf(t, f))-1]; last.Kind != relay.Question || last.Body != "And the lock?" || last.To != handle {
		t.Errorf("tracks review --ask didn't post its question: %+v", last)
	}
	if err := f.svc.ReviewReply(ctx, "r", run, "It's held briefly."); err != nil {
		t.Fatal(err)
	}
	if answer, err := f.svc.WaitReview(ctx, "r"); err != nil || answer != "It's held briefly." {
		t.Fatalf("the second answer: %q, %v", answer, err)
	}
	if unread, err := f.svc.RelayRead(ctx, "r", relay.TrackHandle); err != nil || len(unread) != 0 {
		t.Errorf("the second printed answer is still unread: %+v, %v", unread, err)
	}

	run2 := startReview(t, f, worktree)
	second := oneTrain(t, f).Handle
	if second == handle || run2 == run {
		t.Errorf("a new review kept the handle %s", second)
	}
	msgs = relayOf(t, f)
	left, joined := msgs[len(msgs)-2], msgs[len(msgs)-1]
	if left.Body != "@"+handle+" left: a new review replaces it." || !strings.HasPrefix(joined.Body, "@"+second+" joined") {
		t.Errorf("a new review's notices: %q, %q", left.Body, joined.Body)
	}

	if _, err := f.svc.EndTrain(ctx, "r", "reviewer"); err != nil {
		t.Fatal(err)
	}
	msgs = relayOf(t, f)
	if last := msgs[len(msgs)-1]; last.Body != "@"+second+" left: it was ended." || last.Kind != relay.Notice {
		t.Errorf("ending it: %+v", last)
	}
	if _, err := f.svc.CloseReviewer(ctx, "r"); err != nil {
		t.Fatal(err)
	}
	if again := relayOf(t, f); len(again) != len(msgs) {
		t.Errorf("closing a reviewer that isn't there posted %+v", again[len(msgs):])
	}
}
