package git

import (
	"context"
	"strings"
)

// WorktreeClient operates inside a worktree that `tracks` itself
// created. Unlike PrimaryRepoClient, anything goes here — these
// trees are owned exclusively by `tracks`, and Cursor never points
// at them.
type WorktreeClient struct {
	// Path is the absolute path of the worktree.
	Path string
	// Runner shells out to git inside Path.
	Runner Runner
}

// NewWorktreeClient constructs a client rooted at path.
func NewWorktreeClient(path string) *WorktreeClient {
	return &WorktreeClient{
		Path:   path,
		Runner: ExecRunner{Dir: path},
	}
}

// CurrentBranch returns the branch currently checked out in the
// worktree.
func (c *WorktreeClient) CurrentBranch(ctx context.Context) (string, error) {
	out, _, err := c.Runner.Run(ctx, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// HEAD returns the commit SHA at HEAD.
func (c *WorktreeClient) HEAD(ctx context.Context) (string, error) {
	out, _, err := c.Runner.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// InitSubmodules runs `git submodule update --init --recursive` in
// the worktree. Opt-in per repo via config (init_submodules), since
// it can take minutes per large worktree.
func (c *WorktreeClient) InitSubmodules(ctx context.Context) error {
	_, _, err := c.Runner.Run(ctx, "submodule", "update", "--init", "--recursive")
	return err
}

// HasCommitsBeyond reports whether the worktree's HEAD is ahead of
// base (e.g. "origin/develop"). Useful for `tracks gc --branches`,
// which prunes branches that Claude never actually committed to.
func (c *WorktreeClient) HasCommitsBeyond(ctx context.Context, base string) (bool, error) {
	out, _, err := c.Runner.Run(ctx, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// DirtyFiles returns the porcelain status of the worktree. Empty
// slice = clean tree. Each entry is one line of `git status
// --porcelain`. Not parsed further — the dashboard just needs to
// show a "dirty" badge.
func (c *WorktreeClient) DirtyFiles(ctx context.Context) ([]string, error) {
	out, _, err := c.Runner.Run(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
