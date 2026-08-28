package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/state"
)

// reviewTrack is a track sitting on an open PR with its worktree still
// on disk, so a resume of it skips the restore step and goes straight to
// the spawn — which fails against the test's nonexistent tmux session.
func reviewTrack(t *testing.T, id string) state.Track {
	t.Helper()
	// No gh on PATH: a re-adopted PR watcher polls immediately, and a unit
	// test must not shell out to GitHub for that.
	t.Setenv("PATH", t.TempDir())
	exited := time.Date(2026, 8, 27, 19, 3, 0, 0, time.UTC)
	return state.Track{
		ID:        id,
		Status:    state.StatusPROpen,
		ExitedAt:  &exited,
		SessionID: "88888888-8888-8888-8888-888888888888",
		Kind:      state.KindWork,
		Branch:    "feat/reviewed",
		Slug:      "reviewed",
		PRs:       []state.PRRef{{URL: "https://example.test/pr/1", State: "OPEN"}},
		Repos:     []state.TrackRepo{{Name: "demo", Path: t.TempDir(), Branch: "feat/reviewed"}},
	}
}

// A track in review is resumable. Claude exited when the PR went up, so
// there is a conversation to come back to — and once tracks has been
// restarted there is no window to attach to instead, which used to leave
// the track unreachable: the shutdown sweep skips review tracks and
// `reopen` only takes interrupted ones.
//
// The spawn is what fails here (no tmux session in tests), and that is
// the assertion: the eligibility check and the claim both let a review
// track through.
func TestResumeAcceptsTrackInReview(t *testing.T) {
	srv := newQuietServer(t)
	tr := reviewTrack(t, "in-review")
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	resp := srv.handleResume(context.Background(),
		mustParams(t, ResumeParams{ID: tr.ID}), func(string) {})
	if resp.Ok {
		t.Fatal("resume succeeded; expected the spawn to fail against the test session")
	}
	if !strings.Contains(resp.Error, "spawn claude") {
		t.Errorf("error = %q, want it to come from the spawn step — anything else means the "+
			"track was rejected before it got there", resp.Error)
	}
}

// A resume that fails hands a review track back to review. Errored would
// be wrong twice over: the PR is still open, and Errored is Completed(),
// which puts the worktree in reach of prune-completed and `tracks gc`.
func TestFailedResumeLeavesReviewTrackInReview(t *testing.T) {
	srv := newQuietServer(t)
	tr := reviewTrack(t, "in-review")
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, err := srv.resumeTrackSession(context.Background(), tr, func(string) {}); err == nil {
		t.Fatal("resume succeeded; expected the spawn to fail against the test session")
	}

	got, _ := srv.store.Get(tr.ID)
	if got.Status != state.StatusPROpen {
		t.Errorf("status = %q, want it back at %q", got.Status, state.StatusPROpen)
	}
	if len(got.PRs) != 1 || got.PRs[0].State != "OPEN" {
		t.Errorf("PRs = %+v, want the open PR untouched", got.PRs)
	}
}

// The supervisor a review track carries has no Claude behind it — only a
// PR watcher. Resuming must drop it before spawning, or spawnSupervisor's
// map write would strand that watcher polling on behalf of a supervisor
// nobody owns. When the resume then fails, the watch is handed back with
// the status.
func TestResumeReleasesTheReviewSupervisor(t *testing.T) {
	srv := newQuietServer(t)
	tr := reviewTrack(t, "in-review")
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}
	old := &supervisor{trackID: tr.ID, windowName: tr.WindowName(),
		done: make(chan struct{}), cancel: func() {}}
	srv.supervisors = map[string]*supervisor{tr.ID: old}

	if _, err := srv.resumeTrackSession(context.Background(), tr, func(string) {}); err == nil {
		t.Fatal("resume succeeded; expected the spawn to fail against the test session")
	}

	select {
	case <-old.done:
	default:
		t.Error("the review supervisor was left running; its PR watcher would poll forever")
	}
	srv.mu.Lock()
	cur, ok := srv.supervisors[tr.ID]
	srv.mu.Unlock()
	if !ok {
		t.Fatal("no supervisor after the failed resume; the track is back in review with nothing watching its PR")
	}
	if cur == old {
		t.Error("the dropped supervisor was re-registered; want a fresh one owning the PR watch")
	}
}

// Resume is for tracks with no Claude behind them. A live one has a
// window to attach to, and spawning `claude --resume` on a session that
// is still open would fork the conversation.
func TestResumeRejectsLiveTrack(t *testing.T) {
	srv := newQuietServer(t)
	for _, status := range []state.Status{
		state.StatusPending, state.StatusRunning, state.StatusWaiting, state.StatusDraft,
	} {
		id := string(status)
		if err := srv.store.Put(state.Track{
			ID: id, Status: status,
			SessionID: "99999999-9999-9999-9999-999999999999",
		}); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
		resp := srv.handleResume(context.Background(),
			mustParams(t, ResumeParams{ID: id}), func(string) {})
		if resp.Ok {
			t.Errorf("%s: resume succeeded, want it refused", status)
			continue
		}
		if !strings.Contains(resp.Error, string(status)) {
			t.Errorf("%s: error = %q, want it to name the status", status, resp.Error)
		}
	}
}

// The bug this file exists for: a live track that had opened a PR sat in
// StatusPROpen, so the startup reconcile re-adopted it as review — a PR
// watch with no tmux window and no Claude — and the shutdown sweep had
// already skipped it, so it was never marked interrupted either. It came
// back from a restart with nothing to attach to and nothing to reopen.
func TestReconcileInterruptsLivePROpenTrack(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "live-pr",
		Status:    state.StatusPROpen,
		SessionID: "12121212-1212-1212-1212-121212121212",
		// Spawned and gone with the machine; ExitedAt unset, so Claude was
		// still running when tracks stopped.
		PID: deadPID,
		PRs: []state.PRRef{{URL: "https://example.test/pr/1", State: "OPEN"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.reconcileOnStartup(context.Background())

	got, _ := srv.store.Get("live-pr")
	if got.Status != state.StatusInterrupted {
		t.Fatalf("status = %q, want %q so the track is offered for reopen", got.Status, state.StatusInterrupted)
	}
	if !got.Resumable() {
		t.Error("an interrupted track with a session ID must be resumable")
	}
	srv.mu.Lock()
	_, adopted := srv.supervisors["live-pr"]
	srv.mu.Unlock()
	if adopted {
		t.Error("adopted as a review-only supervisor; that path is for tracks whose claude has exited")
	}
}

// The other road to the same status is untouched: a track whose Claude
// exited on an open PR is still re-adopted into review, keeping its PR
// watch rather than being reopened as a live session.
func TestReconcileReadoptsTrackInReview(t *testing.T) {
	srv := newQuietServer(t)
	tr := reviewTrack(t, "in-review")
	tr.PID = deadPID
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.reconcileOnStartup(context.Background())

	got, _ := srv.store.Get(tr.ID)
	if got.Status != state.StatusPROpen {
		t.Fatalf("status = %q, want it left in review at %q", got.Status, state.StatusPROpen)
	}
	srv.mu.Lock()
	_, adopted := srv.supervisors[tr.ID]
	srv.mu.Unlock()
	if !adopted {
		t.Error("no review supervisor adopted; nothing would watch the open PR")
	}
}

// A stacked-PR track is already StatusPROpen while Claude works on the
// next branch, so the transition into review has no status change to
// make — only the exit stamp, which is what makes the track resumable
// and stops its runtime accruing. Skipping the write on a same-status
// transition would leave it looking live forever.
func TestEnterPRReviewStampsExitOnStackedTrack(t *testing.T) {
	srv := newQuietServer(t)
	t.Setenv("PATH", t.TempDir()) // the PR watcher polls immediately; no gh
	if err := srv.store.Put(state.Track{
		ID:     "stacked",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://example.test/pr/1", State: "OPEN"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	sup := &supervisor{trackID: "stacked", done: make(chan struct{}), cancel: func() {}}

	srv.enterPRReview(sup)

	got, _ := srv.store.Get("stacked")
	if got.ExitedAt == nil {
		t.Fatal("ExitedAt not stamped; the track would read as still running and never become resumable")
	}
	if !got.InReview() {
		t.Error("InReview() false after entering review")
	}
}

// A track finalized out of review had its Claude exit when the PR went
// up; the merge that finalizes it can come days later. Overwriting the
// stamp would report that whole wait as runtime.
func TestFinalizeKeepsTheReviewExitStamp(t *testing.T) {
	srv := newQuietServer(t)
	exited := time.Date(2026, 8, 27, 19, 3, 0, 0, time.UTC)
	if err := srv.store.Put(state.Track{
		ID:        "merged-later",
		Status:    state.StatusPROpen,
		CreatedAt: exited.Add(-time.Hour),
		ExitedAt:  &exited,
		PRs:       []state.PRRef{{URL: "https://example.test/pr/1", State: "MERGED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.finalizeTrack("merged-later")

	got, _ := srv.store.Get("merged-later")
	if got.Status != state.StatusPRMerged {
		t.Fatalf("status = %q, want %q", got.Status, state.StatusPRMerged)
	}
	if got.ExitedAt == nil || !got.ExitedAt.Equal(exited) {
		t.Errorf("ExitedAt = %v, want the original %v", got.ExitedAt, exited)
	}
	if got.Duration() != time.Hour {
		t.Errorf("Duration() = %v, want 1h — the wait for the merge is not runtime", got.Duration())
	}
}

// The PR-watcher claim is per-supervisor and was one-shot, which a
// resume turned into a trap: the resumed session is Running, so the
// watcher armed for its carried-over PRs stops on its first poll — and
// with the claim spent, a PR opened later in that same session could
// never arm another one.
func TestPRWatcherRearmsAfterItsPRsSettle(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:     "resumed",
		Status: state.StatusRunning,
		// Settled, so the first poll needs no gh call and ends the watcher.
		PRs: []state.PRRef{{URL: "https://example.test/pr/1", State: "MERGED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	sup := &supervisor{trackID: "resumed", done: make(chan struct{}), cancel: func() {}}
	if !sup.claimPRWatcher() {
		t.Fatal("fresh supervisor refused the first claim")
	}

	srv.runPRWatcher(sup) // returns on the first poll

	if !sup.claimPRWatcher() {
		t.Error("claim still spent; the next PR this session opens would never be polled")
	}
}

// Ending a track by hand must report the same runtime as letting its PR
// merge on its own: a track ended out of review stopped running when
// Claude exited, not when the user closed it. Driven through a
// worktree-less kind, which reaches the end-state tail with nothing to
// remove.
func TestEndTrackKeepsTheReviewExitStamp(t *testing.T) {
	srv := newQuietServer(t)
	exited := time.Date(2026, 8, 27, 19, 3, 0, 0, time.UTC)
	if err := srv.store.Put(state.Track{
		ID:        "ended-from-review",
		Kind:      state.KindAsk,
		Status:    state.StatusPROpen,
		CreatedAt: exited.Add(-time.Hour),
		ExitedAt:  &exited,
		PRs:       []state.PRRef{{URL: "https://example.test/pr/1", State: "OPEN"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	resp := srv.endTrack(context.Background(),
		mustParams(t, DoneParams{ID: "ended-from-review"}), false, func(string) {})
	if !resp.Ok {
		t.Fatalf("endTrack failed: %s", resp.Error)
	}

	got, _ := srv.store.Get("ended-from-review")
	if !got.Status.IsTerminal() {
		t.Fatalf("status = %q, want an end state", got.Status)
	}
	if got.ExitedAt == nil || !got.ExitedAt.Equal(exited) {
		t.Errorf("ExitedAt = %v, want the original %v", got.ExitedAt, exited)
	}
	if got.Duration() != time.Hour {
		t.Errorf("Duration() = %v, want 1h — the time in review is not runtime", got.Duration())
	}
}
