package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// TestNewPersistsErroredTrackOnCreateFailure verifies that when track
// creation fails partway through (here: the repo path doesn't exist, so
// the branch-collision/fetch step fails — the same shape as a git fetch
// dying during a network drop), the daemon persists an errored track
// carrying the prompt and the failure reason, rather than leaving no
// trace. That's what makes the failure visible and retryable from the
// dashboard.
func TestNewPersistsErroredTrackOnCreateFailure(t *testing.T) {
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/nonexistent/demo", Base: "main"}}
	store := state.NewMemoryStore()
	srv := NewServer(cfg, store, "test")

	raw, err := json.Marshal(NewParams{Repos: []string{"demo"}, TaskPrompt: "fix the thing", Kind: "work"})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(context.Background(), raw, func(string) {})
	if resp.Ok {
		t.Fatal("expected creation to fail against a nonexistent repo, got ok")
	}

	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 persisted track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Status != state.StatusErrored {
		t.Errorf("status = %q, want %q", tr.Status, state.StatusErrored)
	}
	if tr.ErrorMsg == "" {
		t.Error("ErrorMsg is empty; want the failure reason")
	}
	if tr.TaskPrompt != "fix the thing" {
		t.Errorf("TaskPrompt = %q, want it preserved for retry", tr.TaskPrompt)
	}
	if tr.ExitedAt == nil {
		t.Error("ExitedAt is nil; want it set on an errored track")
	}
}
