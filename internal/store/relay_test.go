package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
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

	if moved, err := s.MarkRelayRead(ctx, "a", "track", notice.ID); err != nil || !moved {
		t.Fatalf("marking the notice read: moved %v, %v", moved, err)
	}
	if got := ids("track"); len(got) != 1 || got[0] != report.ID {
		t.Errorf("after reading the notice: %v, want the report", got)
	}
	if moved, err := s.MarkRelayRead(ctx, "a", "track", question.ID); err != nil || moved {
		t.Fatalf("marking an earlier message read: moved %v, %v", moved, err)
	}
	if got := ids("track"); len(got) != 1 {
		t.Errorf("a reader went back: %v", got)
	}
	if moved, _ := s.MarkRelayRead(ctx, "a", "track", notice.ID); moved {
		t.Error("marking the same message read again moved the cursor")
	}

	log, err := s.RelayLog(ctx, "a")
	if err != nil || len(log) != 4 || log[0].ID != question.ID || log[3].Body != "Green yet?" {
		t.Errorf("RelayLog = %+v, %v; want a's four messages, oldest first", log, err)
	}
	unseen := func(reader string) int {
		t.Helper()
		n, err := s.RelayUnseen(ctx, "a", reader)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := unseen(relay.UserHandle); n != 4 {
		t.Errorf("the user hasn't seen %d messages, want all 4", n)
	}
	if _, err := s.MarkRelayRead(ctx, "a", relay.UserHandle, report.ID); err != nil {
		t.Fatal(err)
	}
	if n := unseen(relay.UserHandle); n != 1 {
		t.Errorf("after seeing up to the report, %d unseen; want 1", n)
	}
	if n := unseen("track"); n != 2 {
		t.Errorf("the user's mark moved the track's: %d past its cursor, want 2", n)
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

func TestRelayOutcomeAndRefs(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	if err := s.AddTrack(ctx, track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	refs := relay.Refs{PRs: []string{"https://github.com/acme/api/pull/12"}, Files: []string{"internal/relay/relay.go"}}
	with, err := s.AddRelayMessage(ctx, "a", relay.Message{From: "side-1", To: "track", Kind: relay.Report, Body: "Done.",
		Outcome: relay.Done, Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	without, err := s.AddRelayMessage(ctx, "a", relay.Message{From: "track", To: "side-1", Kind: relay.Question, Body: "Why?"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.RelayMessage(ctx, "a", with.ID)
	if err != nil || got.Outcome != relay.Done || !reflect.DeepEqual(got.Refs, refs) {
		t.Errorf("read back %+v, %v", got, err)
	}
	got, err = s.RelayMessage(ctx, "a", without.ID)
	if err != nil || got.Outcome != "" || !got.Refs.Empty() || got.Refs.PRs != nil {
		t.Errorf("a message without fields read back %+v, %v", got, err)
	}
	log, err := s.RelayLog(ctx, "a")
	if err != nil || len(log) != 2 || log[0].Outcome != relay.Done || !reflect.DeepEqual(log[0].Refs, refs) {
		t.Errorf("the log %+v, %v", log, err)
	}
	var stored string
	if err := s.db.QueryRowContext(ctx, "SELECT refs FROM relay_messages WHERE id = ?", without.ID).Scan(&stored); err != nil || stored != "" {
		t.Errorf("a message without refs stored %q, %v", stored, err)
	}
}
