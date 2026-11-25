package tracks

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// A review can target another checkout of one of a track's repos, such
// as a worktree its agent added for an independent fix. The daemon runs
// no git, since a repo's config can make git run commands, so it tells
// which repo a checkout belongs to by reading its .git.

// checkoutTop is the top folder of the checkout dir is in: the nearest
// folder, dir or above, holding a .git.
func checkoutTop(dir string) (string, bool) {
	d := resolvedPath(dir)
	for {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		up := filepath.Dir(d)
		if up == d {
			return "", false
		}
		d = up
	}
}

// commonGitDir is the git folder every checkout of top's repository
// shares: .git itself for a repo's main checkout; for a linked
// worktree, whose .git is a file naming its own git folder, the one that
// folder's commondir names, once that folder names top back, as git's
// own worktrees do, from under the repository's worktrees folder: a .git
// file, or a git folder beside it, which anyone can write, doesn't make
// a folder a repo's checkout. Git may write either pointer relative to
// the folder that holds it (worktree.useRelativePaths).
func commonGitDir(top string) (string, bool) {
	dotGit := filepath.Join(top, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return resolvedPath(dotGit), true
	}
	text, ok := smallFile(dotGit)
	if !ok {
		return "", false
	}
	gitDir, ok := strings.CutPrefix(text, "gitdir:")
	if !ok {
		return "", false
	}
	gitDir = resolvedPath(against(top, strings.TrimSpace(gitDir)))
	back, ok := smallFile(filepath.Join(gitDir, "gitdir"))
	if !ok || resolvedPath(against(gitDir, back)) != resolvedPath(dotGit) {
		return "", false
	}
	common, ok := smallFile(filepath.Join(gitDir, "commondir"))
	if !ok {
		return "", false
	}
	common = resolvedPath(against(gitDir, common))
	if filepath.Dir(gitDir) != filepath.Join(common, "worktrees") {
		return "", false
	}
	return common, true
}

// against is path, relative to dir when it isn't absolute.
func against(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

// smallFile is the text of the regular file at path, trimmed, when it's
// a few lines at most, as git's pointer files are: a link, a pipe or a
// device read could hang or never end.
func smallFile(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return "", false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// checkoutOf is the repo of t that dir's checkout belongs to, the
// checkout's top folder, and whether that's one of t's worktrees. With
// other it may be any worktree of one of t's repos; without, only one of
// t's own. A checkout nested in one of t's worktrees, such as a worktree
// the agent added under it, is its own, not that worktree.
func checkoutOf(t track.Track, dir string, other bool) (repo track.Repo, top string, own, ok bool) {
	if !filepath.IsAbs(dir) {
		return track.Repo{}, "", false, false
	}
	top, hasTop := checkoutTop(dir)
	if worktree, in := ownWorktree(t, dir); in && (!hasTop || !strictlyIn(top, worktree)) {
		for _, r := range t.Repos {
			if r.Worktree == worktree {
				return r, worktree, true, true
			}
		}
	}
	if !other || !hasTop {
		return track.Repo{}, "", false, false
	}
	common, ok := commonGitDir(top)
	if !ok {
		return track.Repo{}, "", false, false
	}
	for _, r := range t.Repos {
		if r.Worktree == "" {
			continue
		}
		if c, ok := commonGitDir(resolvedPath(r.Worktree)); ok && c == common {
			return r, top, false, true
		}
	}
	return track.Repo{}, "", false, false
}

// strictlyIn says whether path is a folder inside dir, not dir itself.
func strictlyIn(path, dir string) bool {
	return strings.HasPrefix(resolvedPath(path), resolvedPath(dir)+string(filepath.Separator))
}
