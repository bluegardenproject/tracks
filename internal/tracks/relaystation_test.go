package tracks

import (
	"context"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
)

func TestRelayStation(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	changes := &Changes{}
	f.svc.Changes = changes
	f.svc.Store = Watched(f.store, changes)
	ch, _ := changes.Subscribe()
	received(ch)

	notice := post(t, f, relay.Message{From: relay.TracksHandle, Kind: relay.Notice, Body: "@reviewer-1 joined", At: *now})
	report := post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	if n, err := f.svc.RelayNew(ctx, "a"); err != nil || n != 2 {
		t.Fatalf("RelayNew = %d, %v; want 2", n, err)
	}

	log, err := f.svc.RelayStation(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Messages) != 2 || log.Messages[0].ID != notice.ID || log.Messages[1].ID != report.ID {
		t.Fatalf("messages %+v, want the notice and the report", log.Messages)
	}
	if !slices.Equal(log.Waiting, []int64{report.ID}) {
		t.Errorf("waiting %v, want the report alone: a notice wakes no one", log.Waiting)
	}
	if !received(ch) {
		t.Error("the user's mark didn't tell the change feed")
	}
	if n, _ := f.svc.RelayNew(ctx, "a"); n != 0 {
		t.Errorf("after the Relay Station, %d new; want none", n)
	}
	if unread, _ := f.store.RelayUnread(ctx, "a", relay.TrackHandle); len(unread) != 2 {
		t.Errorf("the user's mark read the track's messages: %d unread, want 2", len(unread))
	}
	if _, err := f.svc.RelayStation(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if received(ch) {
		t.Error("showing what was seen already told the change feed again")
	}

	*now = now.Add(pointerGrace)
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %v, want the pointer", got)
	}
	if !received(ch) {
		t.Error("the pointer going didn't tell the change feed")
	}
	if log, _ := f.svc.RelayStation(ctx, "a"); len(log.Waiting) != 0 {
		t.Errorf("waiting %v once pointed to, want none", log.Waiting)
	}
	*now = now.Add(pointerStall)
	deliver(t, f)
	if !received(ch) {
		t.Error("a stalled pointer didn't tell the change feed")
	}
	if log, _ := f.svc.RelayStation(ctx, "a"); !slices.Equal(log.Waiting, []int64{report.ID}) {
		t.Errorf("waiting %v after the stall, want the report again", log.Waiting)
	}

	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	if _, err := f.svc.RelayRead(ctx, "a", relay.TrackHandle); err != nil {
		t.Fatal(err)
	}
	if log, _ := f.svc.RelayStation(ctx, "a"); len(log.Waiting) != 0 {
		t.Errorf("waiting %v once read, want none", log.Waiting)
	}

	if _, err := f.svc.RelayStation(ctx, "gone"); err == nil {
		t.Error("a track that's gone has a relay")
	}
}
