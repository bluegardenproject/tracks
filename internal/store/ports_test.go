package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestClaimPorts(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	for _, id := range []string{"a", "b", "c"} {
		if err := s.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(id string) (int, error) { return s.ClaimPorts(ctx, id, 20000, 10, 2) }
	if a, err := claim("a"); err != nil || a != 20000 {
		t.Fatalf("a claimed %d, %v; want 20000", a, err)
	}
	if again, _ := claim("a"); again != 20000 {
		t.Errorf("a claimed again: %d; want its block back", again)
	}
	if b, _ := claim("b"); b != 20010 {
		t.Errorf("b claimed %d; want 20010", b)
	}
	if _, err := claim("c"); !errors.Is(err, ErrNoPorts) {
		t.Fatalf("c with every block held: %v; want ErrNoPorts", err)
	}
	if err := s.SetState(ctx, "a", track.State{ClosedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if c, err := claim("c"); err != nil || c != 20000 {
		t.Errorf("c after a ended: %d, %v; want a's block", c, err)
	}
}
