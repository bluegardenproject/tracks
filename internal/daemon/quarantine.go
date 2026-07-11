package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/git"
)

// QuarantineDirName is the child of the worktree root where track dirs
// that still hold unsaved work are moved instead of being deleted. It
// is deliberately not a valid track id, and GC skips it, so quarantined
// work is never itself garbage-collected.
const QuarantineDirName = "_recovered"

// ReclaimOrphanTrackDir disposes of an orphaned track directory
// (trackDir = <worktreeRoot>/<id>, containing one git worktree per
// repo) that has no live state entry.
//
// It never deletes unsaved work: every contained worktree is checked
// for uncommitted changes or unpushed commits first (see
// git.WorktreeClient.UnsavedWork). If ANY has unsaved work — or its
// state can't be determined — the whole track dir is *moved* to
// <worktreeRoot>/_recovered/<id> and (quarantined=true, reason) is
// returned; the files (and the branches) survive for the user to
// recover. Only a fully clean, fully pushed track dir is removed via
// `git worktree remove --force` + rm, matching the previous behaviour.
//
// Callers should run `git worktree prune` on their primaries afterwards
// to clear the now-stale worktree admin entries (both GC paths already
// do). This is the safety net for "never delete work out from under a
// live session" (ROADMAP Reliability A).
func ReclaimOrphanTrackDir(ctx context.Context, worktreeRoot, id string) (quarantined bool, reason string, err error) {
	trackDir := filepath.Join(worktreeRoot, id)
	repoEntries, err := os.ReadDir(trackDir)
	if err != nil {
		return false, "", err
	}

	// Decide first: quarantine if any contained worktree has unsaved
	// work, or if we can't verify it (unknown ⇒ preserve).
	for _, re := range repoEntries {
		if !re.IsDir() {
			continue
		}
		wt := filepath.Join(trackDir, re.Name())
		c := git.NewWorktreeClient(wt)
		r, uerr := c.UnsavedWork(ctx)
		if uerr != nil {
			reason = fmt.Sprintf("%s: could not verify (%v)", re.Name(), uerr)
			break
		}
		if r != "" {
			reason = fmt.Sprintf("%s: %s", re.Name(), r)
			break
		}
	}

	if reason != "" {
		// Quarantine: move the whole track dir aside. We deliberately
		// do NOT `git worktree remove` first — that would delete the
		// very files we're preserving. The move leaves git's admin
		// entry pointing at the vanished path; the caller's prune
		// clears it.
		dst := uniquePath(filepath.Join(worktreeRoot, QuarantineDirName, id))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return false, reason, err
		}
		if err := os.Rename(trackDir, dst); err != nil {
			return false, reason, err
		}
		return true, reason, nil
	}

	// Safe to delete: clean tree, everything pushed.
	for _, re := range repoEntries {
		wt := filepath.Join(trackDir, re.Name())
		c := git.WorktreeClient{Path: wt, Runner: git.ExecRunner{Dir: wt}}
		_, _, _ = c.Runner.Run(ctx, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}
	_ = os.RemoveAll(trackDir)
	return false, "", nil
}

// uniquePath returns p if it doesn't exist, else p-1, p-2, … so a
// second quarantine of the same track id doesn't clobber the first.
func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s-%d", p, i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}
