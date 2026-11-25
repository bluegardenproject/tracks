package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestInstructions(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)
	if err := s.AddTrack(ctx, track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	add := func(kind, pr, body string) {
		t.Helper()
		if err := s.AddInstruction(ctx, Instruction{TrackID: "a", Kind: kind, PR: pr, Body: body, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	add("ci_failed", "pr1", "first")
	add("conflicts", "pr1", "rebase")
	add("ci_failed", "pr1", "second")
	add("ci_failed", "pr2", "other")
	waiting, err := s.WaitingInstructions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, in := range waiting {
		bodies = append(bodies, in.Body)
	}
	if len(bodies) != 3 || bodies[0] != "rebase" || bodies[1] != "second" || bodies[2] != "other" {
		t.Fatalf("waiting %q; want the newer of the same kind and PR only, in order", bodies)
	}
	if err := s.InstructionTyped(ctx, waiting[0].ID, now); err != nil {
		t.Fatal(err)
	}
	add("conflicts", "pr1", "again")
	if waiting, _ = s.WaitingInstructions(ctx); len(waiting) != 3 || waiting[2].Body != "again" {
		t.Errorf("after one was typed, a new one of its kind waits: %+v", waiting)
	}
	if still, err := s.InstructionWaiting(ctx, waiting[0].ID); err != nil || !still {
		t.Errorf("a waiting one: %v, %v", still, err)
	}
	if err := s.DropInstructions(ctx, "pr1", "ci_failed", "conflicts"); err != nil {
		t.Fatal(err)
	}
	if waiting, _ = s.WaitingInstructions(ctx); len(waiting) != 1 || waiting[0].Body != "other" {
		t.Errorf("after dropping pr1's: %+v; want pr2's left", waiting)
	}
	var typed int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM instructions WHERE typed_at IS NOT NULL").Scan(&typed); err != nil || typed != 1 {
		t.Errorf("typed rows %d, %v; want the typed one kept", typed, err)
	}
}
