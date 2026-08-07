package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/state"
)

// interruptedByQuit is the note left on a track that was still live
// when tracks was deliberately shut down (menu → Quit session, the
// tmux session killed, or the daemon SIGTERMed).
const interruptedByQuit = "tracks was shut down while this track was still running"

// interruptedUnclean is the note left on a track the previous daemon
// never got to finalize — it died without running its shutdown sweep
// (SIGKILL, a crash, or the machine sleeping).
const interruptedUnclean = "tracks stopped unexpectedly while this track was still running"

// markInterruptedOnShutdown is called from Server.Stop, right after the
// supervisors have been torn down, to record *why* every still-live
// track stopped: tracks went away, the track didn't fail. Without this
// the state file would just say "running" and the next start could only
// guess (see reconcileOnStartup) — and the user would be told their
// work errored when all they did was quit.
//
// StatusPROpen tracks are deliberately left alone: Claude already exited on
// those and reconcileOnStartup re-adopts them into review, which is the
// truthful state to come back to. Drafts have no process at all.
func (s *Server) markInterruptedOnShutdown() {
	now := time.Now().UTC()
	for _, t := range s.store.All() {
		if !sweepable(t) {
			continue
		}
		_, _, _ = s.store.Update(t.ID, func(t *state.Track) bool {
			if !sweepable(*t) {
				return false
			}
			// Only a track Claude actually ran in can be resumed. A track
			// cut down mid-creation has a session id but no conversation
			// behind it, so Interrupted would offer a reopen that spawns
			// `claude --resume` on an empty session — Errored is the honest
			// state. Its saved Draft is usually the way back, though a
			// creation killed before handleNew's own failure path ran won't
			// have one.
			if t.PID > 0 {
				t.Status = state.StatusInterrupted
				t.ErrorMsg = interruptedByQuit
			} else {
				t.Status = state.StatusErrored
				t.ErrorMsg = creationInterrupted
			}
			t.ExitedAt = &now
			return true
		})
	}
}

// creationInterrupted is the note left on a track that was still being
// created (no Claude spawned yet) when tracks went away.
const creationInterrupted = "tracks was shut down while this track was still being created"

// sweepable reports whether the shutdown sweep owns this track's end
// state. Terminal tracks are already settled, a draft has no process,
// and a track in review is re-adopted on the next start.
func sweepable(t state.Track) bool {
	return !t.Status.IsTerminal() &&
		t.Status != state.StatusDraft &&
		t.Status != state.StatusPROpen
}

// reconcileOnStartup is called once during Server.Start, before
// accepting any requests. It does two things:
//
//  1. Settles every track the previous daemon left non-terminal. A
//     clean shutdown already marked those Interrupted (see
//     markInterruptedOnShutdown), so anything still "running" here
//     means the previous daemon died without sweeping: the track is
//     recorded Interrupted too, with a note saying so. The exception is
//     a track whose PID is somehow still alive — we can't re-supervise
//     a process across restarts, so that one is Errored and the user is
//     told how to kill it.
//
//  2. Garbage-collects worktree directories that no longer have a
//     corresponding state entry, in case the daemon crashed
//     mid-rollback.
//
// Logged on stderr so a user reading the daemon's first lines knows
// what happened.
func (s *Server) reconcileOnStartup(ctx context.Context) {
	for _, t := range s.store.All() {
		if t.Status.IsTerminal() {
			continue
		}
		// A saved draft has no worktree and no Claude process — it's just
		// stored parameters waiting to be launched. Leave it untouched.
		if t.Status == state.StatusDraft {
			continue
		}
		// A track left in review (Claude already exited) has no Claude
		// process to re-supervise, so it must never be marked Errored.
		// Its dev servers were orphaned by the dead daemon, so free them
		// first either way.
		if t.Status == state.StatusPROpen {
			if len(t.Services) > 0 {
				t.Services = stopPersistedServices(t.Services, true)
			}
			if t.HasOpenPR() {
				// Still open — keep it in review and re-arm its PR watcher.
				_ = s.store.Put(t)
				s.resumePRReview(t)
			} else {
				// Every PR merged/closed during the downtime — finalize.
				t.Status = terminalStatusFor(t)
				now := time.Now().UTC()
				t.ExitedAt = &now
				_ = s.store.Put(t)
			}
			continue
		}
		alive := t.PID > 0 && processAlive(t.PID)
		// Kill any dev servers orphaned by the previous daemon. They run
		// in their own process groups (not the daemon's), so a daemon
		// crash leaves them bound to their ports; SIGKILL by the stored
		// PGID frees them. Done before marking errored so the persisted
		// Services reflect the teardown.
		if len(t.Services) > 0 {
			t.Services = stopPersistedServices(t.Services, true)
		}
		now := time.Now().UTC()
		t.ExitedAt = &now
		switch {
		case alive:
			t.Status = state.StatusErrored
			t.ErrorMsg = fmt.Sprintf("orphaned by a daemon restart while still running (PID %d) — the daemon can't re-supervise a process across restarts", t.PID)
		case t.PID == 0:
			// Never spawned — cut down mid-creation, so there's no
			// conversation to come back to (see markInterruptedOnShutdown).
			t.Status = state.StatusErrored
			t.ErrorMsg = creationInterrupted
		default:
			// Nothing is running and nothing failed: tracks went away
			// mid-conversation. Interrupted keeps it resumable and keeps it
			// out of any prune-completed sweep.
			t.Status = state.StatusInterrupted
			t.ErrorMsg = interruptedUnclean
		}
		_ = s.store.Put(t)
		switch t.Status {
		case state.StatusInterrupted:
			fmt.Fprintf(os.Stderr,
				"tracks daemon: track %s was still running when tracks last stopped; marked interrupted (bring it back with `tracks reopen`)\n", t.ID)
		case state.StatusErrored:
			if alive {
				fmt.Fprintf(os.Stderr,
					"tracks daemon: track %s had non-terminal status with live PID %d (orphaned from previous daemon); marked errored. To kill the process, run: kill %d\n",
					t.ID, t.PID, t.PID)
			} else {
				fmt.Fprintf(os.Stderr,
					"tracks daemon: track %s never finished being created; marked errored\n", t.ID)
			}
		}
	}

	if err := s.gcOrphanedWorktrees(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "tracks daemon: gc orphans: %v\n", err)
	}
}

// resumePRReview re-attaches a PR watcher to a track the previous daemon
// left in review. There is no Claude process to supervise (it exited
// before the restart), so this registers a minimal review-only
// supervisor whose only job is to keep polling the track's PRs and
// refreshing usage until they merge/close (or the user ends the track).
func (s *Server) resumePRReview(t state.Track) {
	if len(t.PRs) == 0 {
		return
	}
	sup := &supervisor{
		trackID:    t.ID,
		windowName: t.WindowName(),
		done:       make(chan struct{}),
		cancel:     func() {},
	}
	s.mu.Lock()
	if s.supervisors == nil {
		s.supervisors = make(map[string]*supervisor)
	}
	s.supervisors[t.ID] = sup
	s.mu.Unlock()
	s.startPRWatcher(sup)
	fmt.Fprintf(os.Stderr,
		"tracks daemon: track %s left in review; resumed PR watch on %d PR(s)\n",
		t.ID, len(t.PRs))
}

// processAlive reports whether the given PID is still a valid
// process the current user can signal. Uses kill(pid, 0) — the
// classic POSIX liveness check.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix os.FindProcess always succeeds; we have to send a
	// signal to verify.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

// gcOrphanedWorktrees walks <state_dir>/worktrees/<id>/ and removes
// any track-id directory that has no corresponding state entry.
// Worktrees are removed via `git worktree remove --force` so git's
// internal admin (`.git/worktrees/<id>`) is also cleaned up; if
// that fails, we fall back to rm + `git worktree prune`.
func (s *Server) gcOrphanedWorktrees(ctx context.Context) error {
	stateDir, err := s.config().ResolveStateDir()
	if err != nil {
		return err
	}
	worktreeRoot := filepath.Join(stateDir, "worktrees")
	entries, err := os.ReadDir(worktreeRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	known := make(map[string]struct{})
	for _, t := range s.store.All() {
		known[t.ID] = struct{}{}
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Never GC the quarantine dir — it holds preserved unsaved work.
		if e.Name() == QuarantineDirName {
			continue
		}
		if _, ok := known[e.Name()]; ok {
			continue
		}
		// Unknown — reclaim it, but never delete unsaved work: a track
		// dir with uncommitted changes or unpushed commits is moved to
		// worktrees/_recovered/<id> instead of being removed.
		quarantined, reason, err := ReclaimOrphanTrackDir(ctx, worktreeRoot, e.Name())
		if err != nil {
			fmt.Fprintf(os.Stderr, "tracks daemon: gc %s: %v\n", e.Name(), err)
			continue
		}
		if quarantined {
			fmt.Fprintf(os.Stderr,
				"tracks daemon: PRESERVED orphan track %s (%s) — moved to worktrees/%s/%s instead of deleting\n",
				e.Name(), reason, QuarantineDirName, e.Name())
		} else {
			fmt.Fprintf(os.Stderr, "tracks daemon: gc removed orphan track dir %s\n", filepath.Join(worktreeRoot, e.Name()))
		}
	}

	// Run prune on every configured primary to clean up git's
	// internal admin entries.
	for _, r := range s.config().Repos {
		path, err := r.ResolveRepoPath()
		if err != nil {
			continue
		}
		c := git.NewPrimaryRepoClient(path)
		_ = c.PruneWorktrees(ctx)
	}
	return nil
}
