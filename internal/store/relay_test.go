package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestRelay(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	for _, id := range []string{"a", "b"} {
		if err := s.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id string, m relay.Message) relay.Message {
		t.Helper()
		m, err := s.AddRelayMessage(ctx, id, m)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	question := add("a", relay.Message{From: "track", To: "reviewer-1", Kind: relay.Question, Body: "Why?"})
	notice := add("a", relay.Message{From: "tracks", Kind: relay.Notice, Body: "@watch-2 joined"})
	report := add("a", relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine."})
	add("a", relay.Message{From: "track", To: "watch-2", Kind: relay.Question, Body: "Green yet?"})
	add("b", relay.Message{From: "reviewer-9", To: "track", Kind: relay.Report, Body: "Other track."})
	if question.ID == 0 || question.At.IsZero() {
		t.Fatalf("added %+v, want an ID and a time", question)
	}

	ids := func(reader string) []int64 {
		t.Helper()
		msgs, err := s.RelayUnread(ctx, "a", reader)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, m := range msgs {
			out = append(out, m.ID)
		}
		return out
	}
	if got := ids("track"); len(got) != 2 || got[0] != notice.ID || got[1] != report.ID {
		t.Errorf("the track's unread: %v, want the notice and the report, not what it sent or b's", got)
	}
	if got := ids("reviewer-1"); len(got) != 2 || got[0] != question.ID || got[1] != notice.ID {
		t.Errorf("the reviewer's unread: %v, want its question and the notice", got)
	}

	if err := s.MarkRelayRead(ctx, "a", "track", notice.ID); err != nil {
		t.Fatal(err)
	}
	if got := ids("track"); len(got) != 1 || got[0] != report.ID {
		t.Errorf("after reading the notice: %v, want the report", got)
	}
	if err := s.MarkRelayRead(ctx, "a", "track", question.ID); err != nil {
		t.Fatal(err)
	}
	if got := ids("track"); len(got) != 1 {
		t.Errorf("a reader went back: %v", got)
	}

	got, err := s.RelayMessage(ctx, "a", report.ID)
	if err != nil || got.Body != "Fine." || got.Kind != relay.Report || got.From != "reviewer-1" || got.To != "track" {
		t.Errorf("RelayMessage = %+v, %v", got, err)
	}
	if _, err := s.RelayMessage(ctx, "b", report.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another track's message: %v, want ErrNotFound", err)
	}

	if err := s.DeleteTrack(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RelayMessage(ctx, "a", report.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a deleted track's message: %v, want ErrNotFound", err)
	}
	if got := ids("track"); len(got) != 0 {
		t.Errorf("a deleted track's relay: %v", got)
	}
}
