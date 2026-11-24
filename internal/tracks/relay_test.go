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

func TestRelaySend(t *testing.T) {
	ctx := context.Background()
	f, _, worktree := trainsFixture(t, "cursor")
	var problem Problem
	if _, err := f.svc.RelaySend(ctx, "r", "reviewer", relay.Question, "Why?"); !errors.As(err, &problem) {
		t.Errorf("sending to a reviewer that isn't there: %v, want a Problem", err)
	}
	startReview(t, f, worktree)
	for _, tc := range []struct {
		to   string
		kind relay.Kind
		why  string
	}{
		{"reviewer", relay.Question, "hasn't reported yet"},
		{"reviewer", relay.Report, "takes questions"},
		{"detective", relay.Question, `no train called "detective"`},
		{"track", relay.Question, `no train called "track"`},
		{"reviewer", relay.Notice, "isn't a kind of message"},
	} {
		_, err := f.svc.RelaySend(ctx, "r", tc.to, tc.kind, "Why?")
		if !errors.As(err, &problem) || !strings.Contains(err.Error(), tc.why) {
			t.Errorf("sending %s to %s: %v, want a Problem saying %q", tc.kind, tc.to, err, tc.why)
		}
	}
	who, _ := f.svc.RelayWho(ctx, "r")
	if msgs, err := f.store.RelayUnread(ctx, "r", who[1].Handle); err != nil || len(msgs) != 1 || msgs[0].Kind != relay.Notice {
		t.Errorf("refused messages were kept: %+v, %v; want only the reviewer's join notice", msgs, err)
	}
}

func TestRelayReadShowWait(t *testing.T) {
	ctx := context.Background()
	f, _, _ := trainsFixture(t, "cursor")
	report, err := f.store.AddRelayMessage(ctx, "r", relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine."})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := f.svc.RelayRead(ctx, "r", relay.TrackHandle)
	if err != nil || len(msgs) != 1 || msgs[0].ID != report.ID {
		t.Fatalf("read %+v, %v; want the report", msgs, err)
	}
	if msgs, err := f.svc.RelayRead(ctx, "r", relay.TrackHandle); err != nil || len(msgs) != 0 {
		t.Errorf("read again %+v, %v; want nothing new", msgs, err)
	}
	if m, err := f.svc.RelayShow(ctx, "r", relay.TrackHandle, report.ID); err != nil || m.Body != "Fine." {
		t.Errorf("show %+v, %v", m, err)
	}
	var problem Problem
	if _, err := f.svc.RelayShow(ctx, "r", relay.TrackHandle, report.ID+1); !errors.As(err, &problem) {
		t.Errorf("showing a message that isn't there: %v, want a Problem", err)
	}
	if _, err := f.svc.RelayShow(ctx, "r", "watch-2", report.ID); !errors.As(err, &problem) {
		t.Errorf("another train read the reviewer's report: %v", err)
	}
	if _, err := f.svc.RelayRead(ctx, "gone", relay.TrackHandle); !errors.As(err, &problem) {
		t.Errorf("reading a gone track's relay: %v, want a Problem", err)
	}

	got := make(chan []relay.Message, 1)
	go func() {
		msgs, err := f.svc.RelayWait(ctx, "r", relay.TrackHandle)
		if err != nil {
			t.Error(err)
		}
		got <- msgs
	}()
	time.Sleep(2 * relayPoll)
	answer, err := f.store.AddRelayMessage(ctx, "r", relay.Message{From: "reviewer-1", To: "track", Kind: relay.Answer, Body: "Because."})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msgs := <-got:
		if len(msgs) != 1 || msgs[0].ID != answer.ID {
			t.Errorf("waited for %+v, want the answer", msgs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RelayWait didn't see the answer")
	}

	was := relayWait
	relayWait = 50 * time.Millisecond
	defer func() { relayWait = was }()
	if _, err := f.svc.RelayWait(ctx, "r", relay.TrackHandle); !errors.As(err, &problem) {
		t.Errorf("waiting for nothing: %v, want a Problem", err)
	}
}

func TestRelayWho(t *testing.T) {
	ctx := context.Background()
	f, _, worktree := trainsFixture(t, "cursor")
	who, err := f.svc.RelayWho(ctx, "r")
	if err != nil || len(who) != 1 || who[0].Handle != "track" || who[0].Agent != "Track" || who[0].Engine != "Claude Code" || who[0].Status != track.Active {
		t.Fatalf("who before a review: %+v, %v", who, err)
	}
	startReview(t, f, worktree)
	who, err = f.svc.RelayWho(ctx, "r")
	if err != nil || len(who) != 2 || !strings.HasPrefix(who[1].Handle, "reviewer-") || who[1].Agent != "Reviewer" || who[1].Engine != "Cursor (m1)" {
		t.Errorf("who during a review: %+v, %v", who, err)
	}
}
