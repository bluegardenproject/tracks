package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestAgentTokens(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	for _, id := range []string{"a", "b"} {
		if err := s.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetAgentToken(ctx, "nope", "h0"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a token for a missing track: %v, want ErrNotFound", err)
	}
	for id, hash := range map[string]string{"a": "h1", "b": "h2"} {
		if err := s.SetAgentToken(ctx, id, hash); err != nil {
			t.Fatal(err)
		}
	}
	if id, err := s.AgentToken(ctx, "h1"); err != nil || id != "a" {
		t.Errorf("AgentToken(h1) = %q, %v; want a", id, err)
	}
	if err := s.SetAgentToken(ctx, "a", "h3"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentToken(ctx, "h1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a replaced token: %v, want ErrNotFound", err)
	}
	if id, err := s.AgentToken(ctx, "h3"); err != nil || id != "a" {
		t.Errorf("AgentToken(h3) = %q, %v; want a", id, err)
	}
	if err := s.DeleteTrack(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentToken(ctx, "h2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a deleted track's token: %v, want ErrNotFound", err)
	}
}
