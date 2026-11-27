package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// siblingsFixture is sideFixture with a second side track of the main
// track.
func siblingsFixture(t *testing.T) (*fixture, track.Track, track.Track, track.Track) {
	t.Helper()
	f, main, a := sideFixture(t)
	b, err := f.svc.Create(context.Background(), Request{Kind: track.Work, Name: "relay-cli", Prompt: "The relay's commands",
		Parent: main.ID, Side: true, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	return f, main, a, b.Track
}

func TestSideTracksAskEachOther(t *testing.T) {
	ctx := context.Background()
	f, main, a, b := siblingsFixture(t)
	for _, id := range []string{main.ID, a.ID, b.ID} {
		if _, err := f.svc.RelayRead(ctx, id, relay.TrackHandle); err != nil {
			t.Fatal(err)
		}
	}
	q, err := f.svc.RelaySend(ctx, a.ID, "@"+b.Side, relay.Question, "Who owns relay.go?", relay.Fields{})
	if err != nil {
		t.Fatal(err)
	}
	if q.From != a.Side || q.To != b.Side {
		t.Errorf("sent from %s to %s; want from %s to %s", q.From, q.To, a.Side, b.Side)
	}
	later := f.svc.now().Add(time.Minute)
	if due, err := f.svc.dueMessages(ctx, b, 0, later, pointerGrace); err != nil || len(due) != 1 || due[0].ID != q.ID {
		t.Errorf("due for the recipient %+v, %v; want the question", due, err)
	}
	if due, err := f.svc.dueMessages(ctx, main, 0, later, pointerGrace); err != nil || len(due) != 0 {
		t.Errorf("due for the main track %+v, %v; want none: it reads them without a pointer", due, err)
	}
	if due, err := f.svc.dueMessages(ctx, a, 0, later, pointerGrace); err != nil || len(due) != 0 {
		t.Errorf("due for the sender %+v, %v; want none", due, err)
	}
	if log, err := f.svc.RelayStation(ctx, main.ID); err != nil || len(log.Waiting) != 0 {
		t.Errorf("main track's Relay Station waits for %+v, %v; want nothing", log, err)
	}
	if msgs, err := f.svc.RelayRead(ctx, b.ID, relay.TrackHandle); err != nil || len(msgs) != 1 || msgs[0].ID != q.ID {
		t.Errorf("recipient read %+v, %v; want the question", msgs, err)
	}
	if m, err := f.svc.RelayShow(ctx, b.ID, relay.TrackHandle, q.ID); err != nil || m.Body != "Who owns relay.go?" {
		t.Errorf("recipient showed %+v, %v", m, err)
	}
	ans, err := f.svc.RelaySend(ctx, b.ID, a.Side, relay.Answer, "I do.", relay.Fields{})
	if err != nil {
		t.Fatal(err)
	}
	if msgs, err := f.svc.RelayRead(ctx, main.ID, relay.TrackHandle); err != nil || len(msgs) != 2 || msgs[0].ID != q.ID || msgs[1].ID != ans.ID {
		t.Errorf("main track read %+v, %v; want the question and the answer", msgs, err)
	}
	if msgs, err := f.svc.RelayRead(ctx, a.ID, relay.TrackHandle); err != nil || len(msgs) != 1 || msgs[0].ID != ans.ID {
		t.Errorf("sender read %+v, %v; want the answer alone", msgs, err)
	}
}

func TestSideTracksSendEachOtherNoTasks(t *testing.T) {
	ctx := context.Background()
	f, _, a, b := siblingsFixture(t)
	var problem Problem
	for _, kind := range []relay.Kind{relay.Report, relay.Event} {
		_, err := f.svc.RelaySend(ctx, a.ID, b.Side, kind, "Take relay.go.", relay.Fields{})
		if !errors.As(err, &problem) || !strings.Contains(err.Error(), "questions and answers only") || !strings.Contains(err.Error(), "--to track") {
			t.Errorf("a %s to a side track: %v; want a Problem sending it to the main track", kind, err)
		}
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, a.Side, relay.Question, "Me?", relay.Fields{}); !errors.As(err, &problem) {
		t.Errorf("a side track sent to itself: %v, want a Problem", err)
	}
	if m, err := f.svc.RelaySend(ctx, a.ID, "side", relay.Question, "Anyone?", relay.Fields{}); err != nil || m.To != b.Side {
		t.Errorf("the prefix sent %+v, %v; want it to the one other side track", m, err)
	}
}

func TestSideTrackAsksNoEndedSideTrack(t *testing.T) {
	ctx := context.Background()
	f, _, a, b := siblingsFixture(t)
	if err := f.svc.End(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	var problem Problem
	if _, err := f.svc.RelaySend(ctx, a.ID, b.Side, relay.Question, "Still there?", relay.Fields{}); !errors.As(err, &problem) || !strings.Contains(err.Error(), "has ended") {
		t.Errorf("a question to an ended side track: %v; want a Problem", err)
	}
}

func TestSideTracksPairLimit(t *testing.T) {
	ctx := context.Background()
	f, main, a, b := siblingsFixture(t)
	c, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "relay-ui", Prompt: "The Relay Station", Parent: main.ID, Side: true, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	// Both ways count; the main track's messages don't.
	if _, err := f.svc.RelaySend(ctx, main.ID, a.Side, relay.Question, "How's it going?", relay.Fields{}); err != nil {
		t.Fatal(err)
	}
	for i := range relay.PairLimit {
		from, to := a, b
		if i%2 == 1 {
			from, to = b, a
		}
		if _, err := f.svc.RelaySend(ctx, from.ID, to.Side, relay.Question, "Again?", relay.Fields{}); err != nil {
			t.Fatalf("message %d of the pair: %v", i+1, err)
		}
	}
	var problem Problem
	for _, pair := range [][2]track.Track{{a, b}, {b, a}} {
		_, err := f.svc.RelaySend(ctx, pair[0].ID, pair[1].Side, relay.Answer, "Once more.", relay.Fields{})
		if !errors.As(err, &problem) || !strings.Contains(err.Error(), "go through your main track") {
			t.Errorf("past the pair's limit: %v; want a Problem sending it through the main track", err)
		}
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, c.Track.Side, relay.Question, "And you?", relay.Fields{}); err != nil {
		t.Errorf("another pair: %v", err)
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, relay.TrackHandle, relay.Question, "Who owns relay.go?", relay.Fields{}); err != nil {
		t.Errorf("to the main track past the limit: %v", err)
	}
}

func TestSideTrackSeesItsSiblings(t *testing.T) {
	ctx := context.Background()
	f, _, a, b := siblingsFixture(t)
	who, err := f.svc.RelayWho(ctx, a.ID)
	if err != nil || len(who) != 3 || who[0].Handle != relay.TrackHandle || who[1].Handle != a.Side ||
		who[2].Handle != b.Side || who[2].Agent != "Side track relay-cli" {
		t.Errorf("side track sees %+v, %v; want its main track, itself, then the other side track", who, err)
	}
}

func TestATrackStartedFromAnotherKeepsItsSideTracks(t *testing.T) {
	ctx := context.Background()
	f, main, _ := sideFixture(t)
	child, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "spin-off", Repos: []string{"api"}, Prompt: "Elsewhere", Parent: main.ID, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "spin-side", Prompt: "Its own", Parent: child.Track.ID, Side: true, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	who, err := f.svc.RelayWho(ctx, child.Track.ID)
	if err != nil || len(who) != 2 || who[1].Handle != st.Track.Side {
		t.Errorf("a track started from a main track sees %+v, %v; want itself and its own side track", who, err)
	}
	if _, err := f.svc.RelaySend(ctx, child.Track.ID, st.Track.Side, relay.Report, "Your task.", relay.Fields{}); err != nil {
		t.Errorf("it sent its side track a report: %v", err)
	}
}

func TestMainTrackWaitsThroughSideTracksTalking(t *testing.T) {
	ctx := context.Background()
	f, main, a, b := siblingsFixture(t)
	if _, err := f.svc.RelayRead(ctx, main.ID, relay.TrackHandle); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, b.Side, relay.Question, "Who owns relay.go?", relay.Fields{}); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 3*relayPoll)
	defer cancel()
	if msgs, err := f.svc.RelayWait(wait, main.ID, relay.TrackHandle); err == nil {
		t.Errorf("side tracks talking woke the main track: %+v", msgs)
	}
	if msgs, err := f.svc.RelayWait(ctx, b.ID, relay.TrackHandle); err != nil || len(msgs) == 0 || msgs[len(msgs)-1].From != a.Side {
		t.Errorf("the recipient's wait: %+v, %v; want the question", msgs, err)
	}
}

func TestRelaySendOutcomeAndRefs(t *testing.T) {
	ctx := context.Background()
	f, main, a, _ := siblingsFixture(t)
	var problem Problem
	if _, err := f.svc.RelaySend(ctx, a.ID, "track", relay.Report, "Done.", relay.Fields{Refs: []string{"#12"}}); !errors.As(err, &problem) ||
		!strings.Contains(err.Error(), "give the PR's URL") {
		t.Errorf("#12 from a track without PRs: %v, want a Problem asking for the URL", err)
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, "track", relay.Report, "Done.", relay.Fields{Outcome: "great"}); !errors.As(err, &problem) {
		t.Errorf("an unknown outcome: %v, want a Problem", err)
	}
	pr := track.PR{URL: "https://github.com/acme/api/pull/7", Repo: "acme/api", Number: 7, State: track.PROpen}
	if _, err := f.store.AddPR(ctx, a.ID, pr, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	sent, err := f.svc.RelaySend(ctx, a.ID, "track", relay.Report, "Done.",
		relay.Fields{Outcome: relay.Done, Refs: []string{"#12", "internal/relay/relay.go", "https://github.com/acme/api/pull/12"}})
	if err != nil {
		t.Fatal(err)
	}
	log, err := f.store.RelayLog(ctx, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := log[len(log)-1]
	if got.ID != sent.ID || got.Outcome != relay.Done || !slices.Equal(got.Refs.PRs, []string{"https://github.com/acme/api/pull/12"}) ||
		!slices.Equal(got.Refs.Files, []string{"internal/relay/relay.go"}) {
		t.Errorf("as kept: %+v", got)
	}
}
