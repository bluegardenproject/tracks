// Package workspace makes a track's git worktrees, as v1 does: fetch,
// a new branch or a review checkout, and removal when creation fails.
// The user's primary checkouts are only fetched into; their HEAD and
// files never change.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/track"
)

// branchAttempts bounds the -2, -3, … suffixes tried for a branch name
// that is taken.
const branchAttempts = 50

// Worktrees makes worktrees under Root, one folder per track.
type Worktrees struct {
	Root string
	// locks holds a mutex per primary checkout: a review checkout reads
	// FETCH_HEAD, which a second fetch in the same repo would move.
	locks sync.Map
}

// Add checks out t's repos into Root/<id>/<repo> and returns them with
// Worktree and Branch set: a new branch from origin/<base> for a work
// track, the fetched ref, detached, for a review. Other kinds have no
// worktrees and get their repos back unchanged. progress is told each
// slow step. When a step fails, what Add made so far is removed.
func (w *Worktrees) Add(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error) {
	if !t.Kind.Worktrees() {
		return t.Repos, nil
	}
	var review Review
	branch := ""
	if t.Kind == track.Review {
		var err error
		if review, err = ParseReview(t.ReviewRef); err != nil {
			return nil, err
		}
		branch = review.Label
	} else {
		var err error
		if branch, err = freeBranch(ctx, t.Repos, track.Branch(t.ID)); err != nil {
			return nil, err
		}
	}

	made := t
	made.Repos = nil
	for _, r := range t.Repos {
		wtPath, err := worktreePath(w.Root, t.ID, r.Name)
		if err != nil {
			_ = w.Remove(context.WithoutCancel(ctx), made)
			return nil, err
		}
		r.Worktree = wtPath
		r.Branch = branch
		if err := w.add(ctx, r, review, progress); err != nil {
			_ = w.Remove(context.WithoutCancel(ctx), made)
			return nil, err
		}
		made.Repos = append(made.Repos, r)
	}
	return made.Repos, nil
}

func (w *Worktrees) add(ctx context.Context, r track.Repo, review Review, progress func(string)) error {
	defer w.lock(r.Path)()
	primary := git.NewPrimaryRepoClient(r.Path)
	// Reviews diff against origin/<base>, so it is fetched for them too.
	progress(fmt.Sprintf("Fetching origin/%s in %s…", r.Base, r.Name))
	if err := primary.FetchWithRetry(ctx, "origin", r.Base); err != nil {
		return fmt.Errorf("fetch %s in %s: %w", r.Base, r.Name, err)
	}
	if review.Ref == "" {
		progress(fmt.Sprintf("Creating the worktree for %s on %s…", r.Name, r.Branch))
		if err := primary.AddWorktreeWithRetry(ctx, r.Worktree, r.Branch, "origin/"+r.Base); err != nil {
			return fmt.Errorf("create the worktree for %s: %w", r.Name, err)
		}
		return nil
	}
	progress(fmt.Sprintf("Fetching %s in %s…", review.Ref, r.Name))
	if err := primary.FetchWithRetry(ctx, "origin", review.Ref); err != nil {
		return fmt.Errorf("fetch %s in %s: %w", review.Ref, r.Name, err)
	}
	progress(fmt.Sprintf("Checking out %s in %s…", review.Label, r.Name))
	if err := primary.AddWorktreeDetached(ctx, r.Worktree, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("check out %s in %s: %w", review.Label, r.Name, err)
	}
	return nil
}

// AddRepo checks r out into Root/<id>/<repo> for a work track on
// branch, new in r, from origin/<base>. A branch r already has is
// refused rather than checked out: it isn't the track's.
func (w *Worktrees) AddRepo(ctx context.Context, id string, r track.Repo, branch string, progress func(string)) (track.Repo, error) {
	exists, err := git.NewPrimaryRepoClient(r.Path).BranchExists(ctx, branch)
	if err != nil {
		return track.Repo{}, fmt.Errorf("check branch %s in %s: %w", branch, r.Name, err)
	}
	if exists {
		return track.Repo{}, fmt.Errorf("%s already has a branch %s", r.Name, branch)
	}
	wtPath, err := worktreePath(w.Root, id, r.Name)
	if err != nil {
		return track.Repo{}, err
	}
	r.Worktree = wtPath
	r.Branch = branch
	if err := w.add(ctx, r, Review{}, progress); err != nil {
		return track.Repo{}, err
	}
	return r, nil
}

// Remove undoes Add for a track that is being rolled back: its
// worktrees, their folder, and a work track's new branches.
func (w *Worktrees) Remove(ctx context.Context, t track.Track) error {
	var errs []error
	for _, r := range t.Repos {
		if r.Worktree == "" {
			continue
		}
		if w.Root != "" && !withinRoot(w.Root, r.Worktree) {
			errs = append(errs, fmt.Errorf("worktree %q is outside %q: remove it by hand or check XDG_STATE_HOME, then try again", r.Worktree, w.Root))
			continue
		}
		primary := git.NewPrimaryRepoClient(r.Path)
		if err := primary.RemoveWorktree(ctx, r.Worktree); err != nil {
			errs = append(errs, err)
			continue
		}
		if dir, err := trackDir(w.Root, t.ID); err == nil {
			removeEmptyParents(filepath.Dir(r.Worktree), dir)
		}
		if t.Kind == track.Work && r.Branch != "" {
			if err := primary.DeleteBranch(ctx, r.Branch); err != nil {
				errs = append(errs, err)
			}
		}
	}
	// Only an empty folder goes: whatever is left in it isn't ours to
	// delete.
	if dir, err := trackDir(w.Root, t.ID); err == nil {
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\")
}

func repoFolder(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\") {
		return "", fmt.Errorf("invalid repo name %q", name)
	}
	segs := strings.Split(name, "/")
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("invalid repo name %q", name)
		}
	}
	folder := strings.Join(segs, "-")
	if !safeSegment(folder) {
		return "", fmt.Errorf("invalid repo name %q", name)
	}
	return folder, nil
}

func trackDir(root, id string) (string, error) {
	if !safeSegment(id) {
		return "", fmt.Errorf("invalid track id %q", id)
	}
	return filepath.Join(root, id), nil
}

func worktreePath(root, id, name string) (string, error) {
	base, err := trackDir(root, id)
	if err != nil {
		return "", err
	}
	folder, err := repoFolder(name)
	if err != nil {
		return "", err
	}
	candidate := folder
	for n := 2; ; n++ {
		_, err := os.Lstat(filepath.Join(base, candidate))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if n > 100 {
			return "", fmt.Errorf("no free worktree folder for %q", name)
		}
		candidate = fmt.Sprintf("%s-%d", folder, n)
	}
	target := filepath.Join(base, candidate)
	if filepath.Dir(target) != filepath.Clean(base) || filepath.Base(target) != candidate {
		return "", fmt.Errorf("repo name %q escapes worktree directory", name)
	}
	return target, nil
}

// resolveExisting resolves symlinks on path or its deepest existing
// ancestor. It returns false when an existing entry cannot be resolved
// (such as a dangling or looping symlink) so containment checks fail closed.
func resolveExisting(path string) (string, bool) {
	clean := filepath.Clean(path)
	var tail []string
	cur := clean
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Clean(resolved), true
		}
		if _, lerr := os.Lstat(cur); !errors.Is(lerr, os.ErrNotExist) {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return clean, true
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func withinRoot(root, path string) bool {
	rRoot, ok1 := resolveExisting(root)
	rPath, ok2 := resolveExisting(path)
	if !ok1 || !ok2 {
		return false
	}
	rel, err := filepath.Rel(rRoot, rPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// removeEmptyParents removes empty directories between dir and stop (exclusive),
// cleaning up nested folders from older worktrees named with '/'.
func removeEmptyParents(dir, stop string) {
	stop = filepath.Clean(stop)
	for d := filepath.Clean(dir); d != stop && withinRoot(stop, d); d = filepath.Dir(d) {
		if err := os.Remove(d); err != nil {
			return
		}
	}
}

func (w *Worktrees) lock(path string) (unlock func()) {
	m, _ := w.locks.LoadOrStore(path, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// freeBranch is want, or want-2 to want-50, whichever no repo has.
func freeBranch(ctx context.Context, repos []track.Repo, want string) (string, error) {
	for n := 1; n <= branchAttempts; n++ {
		name := want
		if n > 1 {
			name = fmt.Sprintf("%s-%d", want, n)
		}
		taken := false
		for _, r := range repos {
			exists, err := git.NewPrimaryRepoClient(r.Path).BranchExists(ctx, name)
			if err != nil {
				return "", fmt.Errorf("check branch %s in %s: %w", name, r.Name, err)
			}
			if exists {
				taken = true
				break
			}
		}
		if !taken {
			return name, nil
		}
	}
	return "", fmt.Errorf("branch %s and its -2 to -%d suffixes all exist", want, branchAttempts)
}
