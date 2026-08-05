package daemon

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

func TestHasOpenPR(t *testing.T) {
	cases := []struct {
		name string
		tr   state.Track
		want bool
	}{
		{"no url", state.Track{}, false},
		{"url, not yet polled", state.Track{PRURL: "u"}, true},
		{"url open", state.Track{PRURL: "u", PRState: "OPEN"}, true},
		{"url merged", state.Track{PRURL: "u", PRState: "MERGED"}, false},
		{"url closed", state.Track{PRURL: "u", PRState: "CLOSED"}, false},
	}
	for _, c := range cases {
		if got := hasOpenPR(c.tr); got != c.want {
			t.Errorf("%s: hasOpenPR = %v, want %v", c.name, got, c.want)
		}
	}
}

// newQuietServer builds a Server with notifications disabled so
// finalize paths don't touch the OS. It is not Started (no socket, no
// goroutines) — enough to exercise pure state transitions.
//
// Both the state dir and the tmux session name are overridden even
// though these tests don't deliberately touch either: `config.Default()`
// points at the user's real ~/.local/state/tracks and their live `tracks`
// tmux session, so any path or tmux call a server *does* reach (GC,
// sentinels, worktrees, spawning a window) would land on the real thing.
// A daemon test once deleted live worktrees that way, and another
// spawned real Claude sessions into the attached tmux session (Bug 7 in
// the roadmap); this keeps the isolation structural rather than a
// property of what today's tests happen to touch. The HasSession
// assertion mirrors newDocTestServer in docreview_test.go — if the
// generated name somehow resolves, fail loudly instead of spawning.
func newQuietServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Notify.MacOS = false
	cfg.Notify.Bell = false
	cfg.Paths.StateDir = t.TempDir()
	cfg.Tmux.SessionName = "tracks-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	if tmux.New().HasSession(cfg.Tmux.SessionName) {
		t.Fatalf("test tmux session %q exists; refusing to run against a live session", cfg.Tmux.SessionName)
	}
	return NewServer(cfg, state.NewMemoryStore(), "test")
}

// A track in review (StatusPR) is non-terminal, so finalizeTrack must
// still be able to close it out to Done when its PR merges/closes.
func TestFinalizeTrackFromPRReview(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:      "t1",
		Status:  state.StatusPR,
		PRURL:   "https://example.test/pr/1",
		PRState: "MERGED",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.finalizeTrack("t1")

	got, ok := srv.store.Get("t1")
	if !ok {
		t.Fatal("track missing after finalize")
	}
	if got.Status != state.StatusDone {
		t.Errorf("status = %q, want %q", got.Status, state.StatusDone)
	}
	if got.ExitedAt == nil {
		t.Error("ExitedAt not set on finalize")
	}
}

// enterPRReview moves a Claude-exited track with an open PR into
// StatusPR rather than finalizing it to Done, and leaves it there
// (non-terminal) so the worktree survives and usage keeps accruing.
func TestEnterPRReviewKeepsTrackOpen(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:      "t2",
		Status:  state.StatusRunning,
		PRURL:   "https://example.test/pr/2",
		PRState: "OPEN",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A supervisor whose PR watcher is already marked started, so
	// enterPRReview doesn't spawn a gh-polling goroutine in the test.
	sup := &supervisor{trackID: "t2", done: make(chan struct{}), prWatcherStarted: true}

	srv.enterPRReview(sup)

	got, _ := srv.store.Get("t2")
	if got.Status != state.StatusPR {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPR)
	}
	if got.Status.IsTerminal() {
		t.Error("StatusPR must be non-terminal")
	}
}
