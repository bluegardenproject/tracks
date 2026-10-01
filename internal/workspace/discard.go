package workspace

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/track"
)

// Lost is what Discard would lose of t, one entry per repo with some:
// the changed and untracked files in the worktrees still there, and the
// commits on a work track's branches that no other branch and no remote
// has, or for a review, those made in its worktree.
func (w *Worktrees) Lost(ctx context.Context, t track.Track) ([]Unsaved, error) {
	var out []Unsaved
	for _, r := range w.Branches(ctx, t) {
		u := Unsaved{Repo: r.Name}
		if r.Worktree != "" && exists(r.Worktree) {
			var err error
			if u, err = unsaved(ctx, r, t.Kind == track.Review); err != nil {
				return nil, fmt.Errorf("check the worktree for %s: %w", r.Name, err)
			}
		}
		if t.Kind == track.Work && r.Branch != "" {
			n, err := branchOnly(ctx, r)
			if err != nil {
				return nil, fmt.Errorf("check the branch %s in %s: %w", r.Branch, r.Name, err)
			}
			u.Commits = n
		}
		if u.Changed+u.Untracked+u.Commits > 0 {
			out = append(out, u)
		}
	}
	return out, nil
}

// branchOnly counts the commits on r's branch that no other branch and
// no remote has; 0 when the branch is gone.
func branchOnly(ctx context.Context, r track.Repo) (int, error) {
	primary := git.NewPrimaryRepoClient(r.Path)
	if ok, err := primary.BranchExists(ctx, r.Branch); err != nil || !ok {
		return 0, err
	}
	out, _, err := primary.Runner.Run(ctx, "rev-list", "--count", "refs/heads/"+r.Branch,
		"--not", "--exclude="+r.Branch, "--branches", "--remotes")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

// Discard removes t's worktrees, whatever they hold, and a work track's
// local branches. A branch that is a repo's base or checked out in its
// primary checkout stays, and so does whatever was pushed.
func (w *Worktrees) Discard(ctx context.Context, t track.Track) error {
	repos := w.Branches(ctx, t)
	if err := w.RemoveWorktrees(ctx, t.ID, repos); err != nil {
		return err
	}
	if t.Kind != track.Work {
		return nil
	}
	var errs []error
	for _, r := range repos {
		if r.Branch == "" || r.Branch == r.Base {
			continue
		}
		primary := git.NewPrimaryRepoClient(r.Path)
		if current, err := primary.CurrentBranch(ctx); err == nil && current == r.Branch {
			continue
		}
		ok, err := primary.BranchExists(ctx, r.Branch)
		if err == nil && ok {
			err = primary.DeleteBranch(ctx, r.Branch)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("delete the branch %s in %s: %w", r.Branch, r.Name, err))
		}
	}
	return errors.Join(errs...)
}
