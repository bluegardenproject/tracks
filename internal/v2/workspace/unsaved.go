package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Unsaved is the work in one of a track's worktrees that exists nowhere
// else: changed and untracked files, and commits no remote has. A
// review's worktree is detached, so its commits are those made since
// its checkout.
type Unsaved struct {
	Repo               string
	Changed, Untracked int
	Commits            int
}

// String is u for people: "web: 2 changed files and 1 commit that
// exists nowhere else".
func (u Unsaved) String() string {
	var parts []string
	if u.Changed > 0 {
		parts = append(parts, count(u.Changed, "changed file"))
	}
	if u.Untracked > 0 {
		parts = append(parts, count(u.Untracked, "untracked file"))
	}
	if u.Commits == 1 {
		parts = append(parts, "1 commit that exists nowhere else")
	} else if u.Commits > 1 {
		parts = append(parts, strconv.Itoa(u.Commits)+" commits that exist nowhere else")
	}
	list := strings.Join(parts, ", ")
	if i := strings.LastIndex(list, ", "); i >= 0 {
		list = list[:i] + " and " + list[i+2:]
	}
	return u.Repo + ": " + list
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func unsaved(ctx context.Context, r track.Repo, detached bool) (Unsaved, error) {
	wt := git.NewWorktreeClient(r.Worktree)
	u := Unsaved{Repo: r.Name}
	files, err := wt.DirtyFiles(ctx)
	if err != nil {
		return u, err
	}
	for _, f := range files {
		if strings.HasPrefix(f, "??") {
			u.Untracked++
		} else {
			u.Changed++
		}
	}
	args := []string{"rev-list", "--count", "HEAD", "--not", "--remotes"}
	if detached {
		// The oldest entry of the worktree's own HEAD log is its checkout.
		log, _, err := wt.Runner.Run(ctx, "reflog", "--format=%H", "HEAD")
		if err != nil {
			return u, err
		}
		entries := strings.Fields(log)
		if len(entries) == 0 {
			return u, nil
		}
		args = []string{"rev-list", "--count", entries[len(entries)-1] + "..HEAD"}
	}
	out, _, err := wt.Runner.Run(ctx, args...)
	if err != nil {
		return u, err
	}
	u.Commits, err = strconv.Atoi(strings.TrimSpace(out))
	return u, err
}

// Branches are t's repos with each work worktree's branch as it is now,
// since the agent renames the one it started on. A worktree that's
// gone or detached keeps the branch recorded.
func (w *Worktrees) Branches(ctx context.Context, t track.Track) []track.Repo {
	repos := slices.Clone(t.Repos)
	if t.Kind != track.Work {
		return repos
	}
	for i, r := range repos {
		if r.Worktree == "" || !exists(r.Worktree) {
			continue
		}
		if b, err := git.NewWorktreeClient(r.Worktree).CurrentBranch(ctx); err == nil && b != "" {
			repos[i].Branch = b
		}
	}
	return repos
}

// RemoveWorktrees removes repos' worktrees of track id, whatever they
// hold, and then the track's folder if it's empty. Branches stay.
// Worktrees already gone are skipped.
func (w *Worktrees) RemoveWorktrees(ctx context.Context, id string, repos []track.Repo) error {
	var errs []error
	for _, r := range repos {
		if r.Worktree == "" {
			continue
		}
		primary := git.NewPrimaryRepoClient(r.Path)
		if !exists(r.Worktree) {
			_ = primary.PruneWorktrees(ctx)
			continue
		}
		if err := primary.RemoveWorktree(ctx, r.Worktree); err != nil {
			errs = append(errs, fmt.Errorf("remove the worktree for %s: %w", r.Name, err))
		}
	}
	// Only an empty folder goes: whatever is left in it isn't ours to
	// delete.
	dir := filepath.Join(w.Root, id)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		if err := os.Remove(dir); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
