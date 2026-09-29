package workspace

import (
	"context"
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/v2/track"
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
// branch, a review's by fetching its ref again. It returns the repos it
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
		return fmt.Errorf("%s no longer exists in %s", r.Branch, r.Name)
	}
	progress(fmt.Sprintf("Re-creating the worktree for %s on %s…", r.Name, r.Branch))
	if err := primary.CheckoutWorktree(ctx, r.Worktree, r.Branch); err != nil {
		return fmt.Errorf("re-create the worktree for %s: %w", r.Name, err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
