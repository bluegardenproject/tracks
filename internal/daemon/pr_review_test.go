package daemon

import (
	"os"
	"path/filepath"
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
		{"url, not yet polled", state.Track{PRs: []state.PRRef{{URL: "u"}}}, true},
		{"url open", state.Track{PRs: []state.PRRef{{URL: "u", State: "OPEN"}}}, true},
		{"url merged", state.Track{PRs: []state.PRRef{{URL: "u", State: "MERGED"}}}, false},
		{"url closed", state.Track{PRs: []state.PRRef{{URL: "u", State: "CLOSED"}}}, false},
		{"one merged, one open", state.Track{PRs: []state.PRRef{
			{URL: "a", State: "MERGED"}, {URL: "b", State: "OPEN"},
		}}, true},
		{"both merged", state.Track{PRs: []state.PRRef{
			{URL: "a", State: "MERGED"}, {URL: "b", State: "MERGED"},
		}}, false},
	}
	for _, c := range cases {
		if got := c.tr.HasOpenPR(); got != c.want {
			t.Errorf("%s: HasOpenPR = %v, want %v", c.name, got, c.want)
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

// A track in review (StatusPROpen) is non-terminal, so finalizeTrack must
// still be able to close it out when its PR merges — and a merged PR
// settles on StatusPRMerged, not the generic Done.
func TestFinalizeTrackFromPRReview(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:     "t1",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://example.test/pr/1", State: "MERGED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.finalizeTrack("t1")

	got, ok := srv.store.Get("t1")
	if !ok {
		t.Fatal("track missing after finalize")
	}
	if got.Status != state.StatusPRMerged {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPRMerged)
	}
	if got.ExitedAt == nil {
		t.Error("ExitedAt not set on finalize")
	}
}

// A track whose PR was closed without merging has nothing to celebrate:
// it finalizes to plain Done.
func TestFinalizeTrackClosedPRIsDone(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:     "t1c",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://example.test/pr/9", State: "CLOSED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.finalizeTrack("t1c")

	got, _ := srv.store.Get("t1c")
	if got.Status != state.StatusDone {
		t.Errorf("status = %q, want %q", got.Status, state.StatusDone)
	}
}

// A stacked track opens PR #1, keeps working, and #1 merges while Claude
// is still running. The watcher must NOT finalize that track: an end
// state on a live session makes every later state write a no-op, so PR #2
// would never be recorded.
func TestPRTerminalLeavesLiveTrackAlone(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:     "t3",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://example.test/pr/1", State: "MERGED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A sentinel path that doesn't exist = Claude is still running.
	sup := &supervisor{
		trackID:      "t3",
		sentinelPath: filepath.Join(t.TempDir(), "t3.done"),
		done:         make(chan struct{}),
	}
	// Registered, so retire recognises it as the track's current
	// supervisor and actually finalizes once Claude has exited.
	srv.supervisors = map[string]*supervisor{"t3": sup}

	if srv.onPRTerminal(sup) {
		t.Error("onPRTerminal reported done while Claude was still running")
	}
	got, _ := srv.store.Get("t3")
	if got.Status != state.StatusPROpen {
		t.Errorf("status = %q, want %q (still live)", got.Status, state.StatusPROpen)
	}
	if got.ExitedAt != nil {
		t.Error("ExitedAt stamped on a live track")
	}

	// Now Claude exits: the sentinel appears and the track finalizes as
	// a merged PR.
	if err := os.WriteFile(sup.sentinelPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !srv.onPRTerminal(sup) {
		t.Fatal("onPRTerminal did not finish once Claude had exited")
	}
	got, _ = srv.store.Get("t3")
	if got.Status != state.StatusPRMerged {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPRMerged)
	}
}

// enterPRReview moves a Claude-exited track with an open PR into
// StatusPROpen rather than finalizing it, and leaves it there
// (non-terminal) so the worktree survives and usage keeps accruing.
func TestEnterPRReviewKeepsTrackOpen(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:     "t2",
		Status: state.StatusRunning,
		PRs:    []state.PRRef{{URL: "https://example.test/pr/2", State: "OPEN"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A supervisor whose PR watcher is already marked started, so
	// enterPRReview doesn't spawn a gh-polling goroutine in the test.
	sup := &supervisor{trackID: "t2", done: make(chan struct{}), prWatcherStarted: true}

	srv.enterPRReview(sup)

	got, _ := srv.store.Get("t2")
	if got.Status != state.StatusPROpen {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPROpen)
	}
	if got.Status.IsTerminal() {
		t.Error("StatusPROpen must be non-terminal")
	}
}
