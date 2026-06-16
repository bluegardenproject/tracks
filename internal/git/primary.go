package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PrimaryRepoClient operates against a user's primary checkout (the
// repo at <repo.path>). Its method surface is deliberately tiny:
// fetch + worktree add/remove/list/prune + branch existence checks.
// There is NO method that moves the primary checkout's HEAD or
// touches its working tree, because that would yank Cursor onto a
// different branch.
type PrimaryRepoClient struct {
	// Path is the absolute path of the primary checkout.
	Path string
	// Runner shells out to git. Tests inject a fake.
	Runner Runner
}

// NewPrimaryRepoClient constructs a client rooted at path. The
// returned client uses ExecRunner with cwd=path.
func NewPrimaryRepoClient(path string) *PrimaryRepoClient {
	return &PrimaryRepoClient{
		Path:   path,
		Runner: ExecRunner{Dir: path},
	}
}

// Fetch runs `git fetch <remote> <ref> --no-tags`. This is safe for
// the primary checkout: it only writes under .git/refs/remotes/ and
// .git/objects/, never touching HEAD or the working tree.
func (c *PrimaryRepoClient) Fetch(ctx context.Context, remote, ref string) error {
	_, _, err := c.Runner.Run(ctx, "fetch", remote, ref, "--no-tags")
	return err
}

// AddWorktree creates a new worktree at path on a new branch
// branchName, starting from start (a ref like "origin/develop").
//
// This is the central worktree-creation operation in `tracks`. It
// runs:
//
//	git worktree add -b <branchName> <path> <start>
//
// which creates the branch and the worktree in one step. The new
// branch does NOT shadow the primary's HEAD.
//
// If the branch already exists in any worktree, git refuses; the
// caller should pick a different name (typically by appending
// -<n>).
func (c *PrimaryRepoClient) AddWorktree(ctx context.Context, path, branchName, start string) error {
	if path == "" || branchName == "" || start == "" {
		return errors.New("AddWorktree: path/branchName/start required")
	}
	_, _, err := c.Runner.Run(ctx, "worktree", "add", "-b", branchName, path, start)
	return err
}

// AddWorktreeDetached creates a worktree at path on a detached HEAD
// pointing at start (a ref like "FETCH_HEAD" or "origin/feat/x"). No
// branch is created, so the same commit can be checked out here while
// living in another worktree — exactly what a read-only review wants.
//
// It runs:
//
//	git worktree add --detach <path> <start>
func (c *PrimaryRepoClient) AddWorktreeDetached(ctx context.Context, path, start string) error {
	if path == "" || start == "" {
		return errors.New("AddWorktreeDetached: path/start required")
	}
	_, _, err := c.Runner.Run(ctx, "worktree", "add", "--detach", path, start)
	return err
}

// RemoveWorktree removes the worktree at path. --force is used so a
// worktree with uncommitted changes can still be cleaned up at track
// end. The associated branch is NOT deleted (see plan §"Branch
// cleanup": tracks intentionally keeps branches around so the user
// can revisit them).
func (c *PrimaryRepoClient) RemoveWorktree(ctx context.Context, path string) error {
	_, _, err := c.Runner.Run(ctx, "worktree", "remove", "--force", path)
	return err
}

// PruneWorktrees runs `git worktree prune`, which removes worktree
// administrative entries (under .git/worktrees/) whose checkout
// directory no longer exists. Used by `tracks gc`.
func (c *PrimaryRepoClient) PruneWorktrees(ctx context.Context) error {
	_, _, err := c.Runner.Run(ctx, "worktree", "prune")
	return err
}

// Worktree describes one entry from `git worktree list --porcelain`.
type Worktree struct {
	// Path is the absolute filesystem path of the worktree's
	// checkout root.
	Path string
	// HEAD is the commit SHA at the worktree's HEAD.
	HEAD string
	// Branch is the local branch name (without "refs/heads/")
	// checked out in the worktree, or empty if detached.
	Branch string
	// Detached is true when the worktree is on a detached HEAD.
	Detached bool
}

// ListWorktrees parses `git worktree list --porcelain`.
//
// Output format is documented in git-worktree(1): an empty-line
// separated list of records, each line being "key value" or a bare
// keyword like "detached".
func (c *PrimaryRepoClient) ListWorktrees(ctx context.Context) ([]Worktree, error) {
	out, _, err := c.Runner.Run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		out2    []Worktree
		current Worktree
		flushed = true
	)
	flush := func() {
		if !flushed {
			out2 = append(out2, current)
		}
		current = Worktree{}
		flushed = true
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimPrefix(line, "worktree ")
			flushed = false
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			current.Detached = true
		}
	}
	flush()
	return out2, nil
}

// BranchExists reports whether refs/heads/<name> exists locally.
// Uses `show-ref --verify --quiet` so a missing branch returns
// (false, nil), not an error.
func (c *PrimaryRepoClient) BranchExists(ctx context.Context, name string) (bool, error) {
	_, _, err := c.Runner.Run(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true, nil
	}
	if ExitErrorOf(err) == 1 {
		return false, nil
	}
	return false, err
}

// CurrentBranch returns the branch currently checked out at the
// primary. Used for sanity checks ("we never moved Cursor's branch")
// and for surfacing "primary is on X" in the dashboard.
func (c *PrimaryRepoClient) CurrentBranch(ctx context.Context) (string, error) {
	out, _, err := c.Runner.Run(ctx, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DeleteBranch removes a local branch with -D (force, even if not
// merged). `tracks gc --branches` uses this; the default cleanup
// path (`tracks done`) does NOT delete branches.
func (c *PrimaryRepoClient) DeleteBranch(ctx context.Context, name string) error {
	if name == "" {
		return errors.New("DeleteBranch: name required")
	}
	_, _, err := c.Runner.Run(ctx, "branch", "-D", name)
	return err
}

// AddWorktreeWithRetry calls AddWorktree, retrying on transient
// failures (most commonly an index.lock contention with a Cursor
// operation racing us). Backs off 500ms × attempt, up to 3 attempts
// total, before returning the last error.
func (c *PrimaryRepoClient) AddWorktreeWithRetry(ctx context.Context, path, branchName, start string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := c.AddWorktree(ctx, path, branchName, start); err == nil {
			return nil
		} else {
			lastErr = err
			if !isTransient(err) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
	return fmt.Errorf("AddWorktree after 3 attempts: %w", lastErr)
}

// isTransient identifies error messages we expect can succeed if
// retried. We deliberately keep this narrow to avoid masking real
// bugs.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "index.lock") ||
		strings.Contains(msg, "cannot lock ref") ||
		strings.Contains(msg, "fatal: Unable to create")
}
