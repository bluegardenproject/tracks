package daemon

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
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
func newQuietServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Notify.MacOS = false
	cfg.Notify.Bell = false
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
