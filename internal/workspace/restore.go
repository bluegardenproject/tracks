package workspace

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/track"
)

// Missing are t's repos whose worktree is gone.
func (w *Worktrees) Missing(t track.Track) []track.Repo {
	var out []track.Repo
	for _, r := range t.Repos {
		if r.Worktree != "" && !exists(r.Worktree) {
			out = append(out, r)
		}
	}
	return out
}

// Restore re-creates t's worktrees that are gone: a work track's on its
// branch, which is re-created too when it was deleted, a review's by
// fetching its ref again. It returns the repos it
// re-created. When one fails, those it made are removed again.
func (w *Worktrees) Restore(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error) {
	if !t.Kind.Worktrees() {
		return nil, nil
	}
	var review Review
	if t.Kind == track.Review {
		var err error
		if review, err = ParseReview(t.ReviewRef); err != nil {
			return nil, err
		}
	}
	var made []track.Repo
	for _, r := range w.Missing(t) {
		if err := w.restore(ctx, r, review, progress); err != nil {
			_ = w.RemoveWorktrees(context.WithoutCancel(ctx), t.ID, made)
			return nil, err
		}
		made = append(made, r)
	}
	return made, nil
}

func (w *Worktrees) restore(ctx context.Context, r track.Repo, review Review, progress func(string)) error {
	primary := git.NewPrimaryRepoClient(r.Path)
	// A worktree removed by hand leaves its entry behind, which would
	// refuse the same path.
	_ = primary.PruneWorktrees(ctx)
	if review.Ref != "" {
		return w.add(ctx, r, review, progress)
	}
	defer w.lock(r.Path)()
	found, err := primary.BranchExists(ctx, r.Branch)
	if err != nil {
		return fmt.Errorf("check branch %s in %s: %w", r.Branch, r.Name, err)
	}
	if !found {
		return w.rebranch(ctx, primary, r, progress)
	}
	progress(fmt.Sprintf("Re-creating the worktree for %s on %s…", r.Name, r.Branch))
	if err := primary.CheckoutWorktree(ctx, r.Worktree, r.Branch); err != nil {
		return fmt.Errorf("re-create the worktree for %s: %w", r.Name, err)
	}
	return nil
}

// rebranch re-creates r's deleted branch with its worktree: from the
// pushed branch when origin has it, else as a new branch from
// origin/<base>.
func (w *Worktrees) rebranch(ctx context.Context, primary *git.PrimaryRepoClient, r track.Repo, progress func(string)) error {
	heads, _, err := primary.Runner.Run(ctx, "ls-remote", "--heads", "origin", "refs/heads/"+r.Branch)
	if err != nil {
		return fmt.Errorf("look for %s on origin in %s: %w", r.Branch, r.Name, err)
	}
	start := r.Base
	if strings.TrimSpace(heads) != "" {
		start = r.Branch
	}
	progress(fmt.Sprintf("Fetching origin/%s in %s…", start, r.Name))
	if err := primary.FetchWithRetry(ctx, "origin", start); err != nil {
		return fmt.Errorf("fetch %s in %s: %w", start, r.Name, err)
	}
	progress(fmt.Sprintf("Re-creating %s in %s from origin/%s…", r.Branch, r.Name, start))
	if err := primary.AddWorktree(ctx, r.Worktree, r.Branch, "origin/"+start); err != nil {
		return fmt.Errorf("re-create the worktree for %s: %w", r.Name, err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
