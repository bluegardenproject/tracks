package tracks

import (
	"context"
	"errors"
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
	q, err := f.svc.RelaySend(ctx, a.ID, "@"+b.Side, relay.Question, "Who owns relay.go?")
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
	ans, err := f.svc.RelaySend(ctx, b.ID, a.Side, relay.Answer, "I do.")
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
		_, err := f.svc.RelaySend(ctx, a.ID, b.Side, kind, "Take relay.go.")
		if !errors.As(err, &problem) || !strings.Contains(err.Error(), "questions and answers only") || !strings.Contains(err.Error(), "--to track") {
			t.Errorf("a %s to a side track: %v; want a Problem sending it to the main track", kind, err)
		}
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, a.Side, relay.Question, "Me?"); !errors.As(err, &problem) {
		t.Errorf("a side track sent to itself: %v, want a Problem", err)
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, "side", relay.Question, "Anyone?"); !errors.As(err, &problem) {
		t.Errorf("a prefix of two side tracks: %v, want a Problem", err)
	}
}

func TestSideTrackAsksNoEndedSideTrack(t *testing.T) {
	ctx := context.Background()
	f, _, a, b := siblingsFixture(t)
	if err := f.svc.End(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	var problem Problem
	if _, err := f.svc.RelaySend(ctx, a.ID, b.Side, relay.Question, "Still there?"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "has ended") {
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
	if _, err := f.svc.RelaySend(ctx, main.ID, a.Side, relay.Question, "How's it going?"); err != nil {
		t.Fatal(err)
	}
	for i := range relay.PairLimit {
		from, to := a, b
		if i%2 == 1 {
			from, to = b, a
		}
		if _, err := f.svc.RelaySend(ctx, from.ID, to.Side, relay.Question, "Again?"); err != nil {
			t.Fatalf("message %d of the pair: %v", i+1, err)
		}
	}
	var problem Problem
	for _, pair := range [][2]track.Track{{a, b}, {b, a}} {
		_, err := f.svc.RelaySend(ctx, pair[0].ID, pair[1].Side, relay.Answer, "Once more.")
		if !errors.As(err, &problem) || !strings.Contains(err.Error(), "go through your main track") {
			t.Errorf("past the pair's limit: %v; want a Problem sending it through the main track", err)
		}
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, c.Track.Side, relay.Question, "And you?"); err != nil {
		t.Errorf("another pair: %v", err)
	}
	if _, err := f.svc.RelaySend(ctx, a.ID, relay.TrackHandle, relay.Question, "Who owns relay.go?"); err != nil {
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
	if _, err := f.svc.RelaySend(ctx, child.Track.ID, st.Track.Side, relay.Report, "Your task."); err != nil {
		t.Errorf("it sent its side track a report: %v", err)
	}
}
