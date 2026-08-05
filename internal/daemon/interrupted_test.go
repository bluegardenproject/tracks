package daemon

import (
	"context"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// deadPID is above the maximum pid on Linux and macOS, so kill(pid, 0)
// reports ESRCH — a track "that was spawned but is gone" without
// signalling anything real or depending on who we run as.
const deadPID = 0x7FFFFFFF

// A deliberate shutdown records every live track as Interrupted, so the
// next start (and the user) can tell "tracks was quit" apart from "this
// track failed". Drafts, tracks in review, and already-finished tracks
// keep the status they had.
func TestMarkInterruptedOnShutdown(t *testing.T) {
	srv := newQuietServer(t)
	seed := []state.Track{
		{ID: "running", Status: state.StatusRunning, PID: deadPID},
		{ID: "waiting", Status: state.StatusWaiting, PID: deadPID},
		{ID: "pending", Status: state.StatusPending, PID: deadPID},
		{ID: "review", Status: state.StatusPR, PRURL: "https://example.test/pr/1"},
		{ID: "draft", Status: state.StatusDraft},
		{ID: "done", Status: state.StatusDone},
		{ID: "errored", Status: state.StatusErrored, ErrorMsg: "spawn claude: boom"},
	}
	for _, tr := range seed {
		if err := srv.store.Put(tr); err != nil {
			t.Fatalf("put %s: %v", tr.ID, err)
		}
	}

	srv.markInterruptedOnShutdown()

	want := map[string]state.Status{
		"running": state.StatusInterrupted,
		"waiting": state.StatusInterrupted,
		"pending": state.StatusInterrupted,
		"review":  state.StatusPR,
		"draft":   state.StatusDraft,
		"done":    state.StatusDone,
		"errored": state.StatusErrored,
	}
	for id, wantStatus := range want {
		got, ok := srv.store.Get(id)
		if !ok {
			t.Fatalf("track %s missing", id)
		}
		if got.Status != wantStatus {
			t.Errorf("%s: status = %q, want %q", id, got.Status, wantStatus)
		}
		if wantStatus != state.StatusInterrupted {
			continue
		}
		if got.ExitedAt == nil {
			t.Errorf("%s: ExitedAt not set", id)
		}
		if got.ErrorMsg != interruptedByQuit {
			t.Errorf("%s: ErrorMsg = %q, want %q", id, got.ErrorMsg, interruptedByQuit)
		}
	}

	// The errored track's own reason must not be overwritten.
	if got, _ := srv.store.Get("errored"); got.ErrorMsg != "spawn claude: boom" {
		t.Errorf("errored ErrorMsg = %q, want it preserved", got.ErrorMsg)
	}
}

// A track cut down before Claude was ever spawned (PID 0) has a session
// id but no conversation behind it. Offering to reopen it would spawn
// `claude --resume` on an empty session, so it must settle on Errored —
// its saved Draft is the way back.
func TestMarkInterruptedSkipsNeverSpawnedTrack(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "mid-creation",
		Status:    state.StatusPending,
		SessionID: "33333333-3333-3333-3333-333333333333",
		PID:       0,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.markInterruptedOnShutdown()

	got, _ := srv.store.Get("mid-creation")
	if got.Status != state.StatusErrored {
		t.Errorf("status = %q, want %q", got.Status, state.StatusErrored)
	}
	if got.ErrorMsg != creationInterrupted {
		t.Errorf("ErrorMsg = %q, want %q", got.ErrorMsg, creationInterrupted)
	}
}

// Same rule on the startup path: a never-spawned track the previous
// daemon left behind is Errored, not offered for reopen.
func TestReconcileErrorsNeverSpawnedTrack(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "mid-creation",
		Status:    state.StatusPending,
		SessionID: "44444444-4444-4444-4444-444444444444",
		PID:       0,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.reconcileOnStartup(context.Background())

	got, _ := srv.store.Get("mid-creation")
	if got.Status != state.StatusErrored {
		t.Errorf("status = %q, want %q", got.Status, state.StatusErrored)
	}
	if got.ErrorMsg != creationInterrupted {
		t.Errorf("ErrorMsg = %q, want %q", got.ErrorMsg, creationInterrupted)
	}
}

// An interrupted track is terminal (nothing runs) but not Completed, so
// it survives a "clear completed" sweep while done/errored tracks go.
func TestPruneCompletedKeepsInterrupted(t *testing.T) {
	srv := newQuietServer(t)
	for _, tr := range []state.Track{
		{ID: "done", Status: state.StatusDone},
		{ID: "errored", Status: state.StatusErrored},
		{ID: "interrupted", Status: state.StatusInterrupted},
		{ID: "running", Status: state.StatusRunning},
	} {
		if err := srv.store.Put(tr); err != nil {
			t.Fatalf("put %s: %v", tr.ID, err)
		}
	}

	resp := srv.handlePruneCompleted()
	if !resp.Ok {
		t.Fatalf("prune failed: %s", resp.Error)
	}

	for _, id := range []string{"interrupted", "running"} {
		if _, ok := srv.store.Get(id); !ok {
			t.Errorf("%s was pruned; it must be kept", id)
		}
	}
	for _, id := range []string{"done", "errored"} {
		if _, ok := srv.store.Get(id); ok {
			t.Errorf("%s survived prune; it should have been removed", id)
		}
	}
}

// Startup reconciliation of a track the previous daemon never got to
// sweep: with no live process it is Interrupted (resumable), not
// Errored — the user only quit.
func TestReconcileMarksDeadTrackInterrupted(t *testing.T) {
	// newQuietServer isolates StateDir, which matters here:
	// reconcileOnStartup also garbage-collects worktree dirs that have no
	// state entry.
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "orphan",
		Status:    state.StatusRunning,
		SessionID: "11111111-1111-1111-1111-111111111111",
		// Spawned (so it has a conversation to come back to) but long gone.
		PID: deadPID,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.reconcileOnStartup(context.Background())

	got, ok := srv.store.Get("orphan")
	if !ok {
		t.Fatal("track missing after reconcile")
	}
	if got.Status != state.StatusInterrupted {
		t.Fatalf("status = %q, want %q", got.Status, state.StatusInterrupted)
	}
	if got.ErrorMsg != interruptedUnclean {
		t.Errorf("ErrorMsg = %q, want %q", got.ErrorMsg, interruptedUnclean)
	}
	if got.ExitedAt == nil {
		t.Error("ExitedAt not set")
	}
	if !got.Resumable() {
		t.Error("an interrupted track with a session ID must be resumable")
	}
}

// A shutdown-driven pane death must not be recorded as a natural exit:
// with shuttingDown set, the watcher's retire path leaves the track's
// end state to markInterruptedOnShutdown instead of writing Done.
func TestRetireOrReviewSkipsFinalizeDuringShutdown(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{ID: "t1", Status: state.StatusRunning, PID: deadPID}); err != nil {
		t.Fatalf("put: %v", err)
	}
	sup := &supervisor{trackID: "t1", pid: deadPID, done: make(chan struct{})}
	srv.supervisors = map[string]*supervisor{"t1": sup}

	srv.shuttingDown.Store(true)
	srv.retireOrReview(sup)

	got, _ := srv.store.Get("t1")
	if got.Status != state.StatusRunning {
		t.Errorf("status = %q, want it left at %q for the shutdown sweep",
			got.Status, state.StatusRunning)
	}
	select {
	case <-sup.done:
	default:
		t.Error("supervisor not released (sup.done still open)")
	}

	// The sweep then settles it.
	srv.markInterruptedOnShutdown()
	if got, _ := srv.store.Get("t1"); got.Status != state.StatusInterrupted {
		t.Errorf("status after sweep = %q, want %q", got.Status, state.StatusInterrupted)
	}
}
