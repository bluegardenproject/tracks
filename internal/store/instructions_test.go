package store

import (
	"context"
	"errors"
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
	add("ci_failed", "pr1", "only")
	only, _ := s.WaitingInstructions(ctx)
	add("ci_failed", "pr1", "first")
	if still, err := s.InstructionWaiting(ctx, only[0].ID); err != nil || still {
		t.Errorf("a replaced instruction's ID still waits: %v, %v; its replacement took it", still, err)
	}
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

func TestDropInstructionAndSetBase(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)
	if err := s.AddTrack(ctx, track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: now,
		Repos: []track.Repo{{Name: "api", Path: "/src/api", Base: "side/x"}}}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"typed", "waiting"} {
		if err := s.AddInstruction(ctx, Instruction{TrackID: "a", Kind: body, PR: "pr", Body: body, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.WaitingInstructions(ctx)
	if err := s.InstructionTyped(ctx, all[0].ID, now); err != nil {
		t.Fatal(err)
	}
	for _, in := range all {
		if err := s.DropInstruction(ctx, in.ID); err != nil {
			t.Fatal(err)
		}
	}
	var left int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM instructions").Scan(&left); err != nil || left != 1 {
		t.Errorf("%d rows left, %v; want the typed one kept", left, err)
	}

	if err := s.SetBase(ctx, "a", "api", "main"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Track(ctx, "a"); err != nil || got.Repos[0].Base != "main" {
		t.Errorf("base %q, %v; want main", got.Repos[0].Base, err)
	}
	if err := s.SetBase(ctx, "gone", "api", "main"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetBase on a track that isn't there: %v", err)
	}
}
