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

// reconcileOnStartup is called once during Server.Start, before
// accepting any requests. It does two things:
//
//  1. Marks every non-terminal track Errored. We can't re-supervise
//     a Claude process across daemon restarts (no portable way to
//     reattach to a child's exit code), so the safest thing is to
//     acknowledge the gap in observation.
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
		alive := t.PID > 0 && processAlive(t.PID)
		t.Status = state.StatusErrored
		now := time.Now().UTC()
		t.ExitedAt = &now
		_ = s.store.Put(t)
		if alive {
			fmt.Fprintf(os.Stderr,
				"tracks daemon: track %s had non-terminal status with live PID %d (orphaned from previous daemon); marked errored. To kill the process, run: kill %d\n",
				t.ID, t.PID, t.PID)
		} else {
			fmt.Fprintf(os.Stderr,
				"tracks daemon: track %s had non-terminal status with no live process; marked errored\n", t.ID)
		}
	}

	if err := s.gcOrphanedWorktrees(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "tracks daemon: gc orphans: %v\n", err)
	}
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
	stateDir, err := s.cfg.ResolveStateDir()
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
		if _, ok := known[e.Name()]; ok {
			continue
		}
		// Unknown — clean it up.
		trackDir := filepath.Join(worktreeRoot, e.Name())
		repoEntries, err := os.ReadDir(trackDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tracks daemon: gc read %s: %v\n", trackDir, err)
			continue
		}
		for _, repoEntry := range repoEntries {
			worktreePath := filepath.Join(trackDir, repoEntry.Name())
			// We don't know which primary the worktree came from,
			// but `git worktree remove` works from inside the
			// worktree as well. Try that.
			c := git.WorktreeClient{Path: worktreePath, Runner: git.ExecRunner{Dir: worktreePath}}
			_, _, _ = c.Runner.Run(ctx, "worktree", "remove", "--force", worktreePath)
			_ = os.RemoveAll(worktreePath)
		}
		_ = os.RemoveAll(trackDir)
		fmt.Fprintf(os.Stderr, "tracks daemon: gc removed orphan track dir %s\n", trackDir)
	}

	// Run prune on every configured primary to clean up git's
	// internal admin entries.
	for _, r := range s.cfg.Repos {
		path, err := r.ResolveRepoPath()
		if err != nil {
			continue
		}
		c := git.NewPrimaryRepoClient(path)
		_ = c.PruneWorktrees(ctx)
	}
	return nil
}
