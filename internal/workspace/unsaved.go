package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/track"
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

// maxGitFileBytes bounds the size of a worktree .git pointer file.
const maxGitFileBytes = 4096

// primaryGitDir resolves primary's git directory, supporting both a
// standard .git directory and a .git gitfile (e.g. --separate-git-dir).
func primaryGitDir(primary string) (string, error) {
	primaryReal, err := filepath.EvalSymlinks(primary)
	if err != nil {
		return "", fmt.Errorf("resolve primary path: %w", err)
	}
	dotGit := filepath.Join(primaryReal, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", fmt.Errorf("stat primary .git: %w", err)
	}
	if info.IsDir() {
		return filepath.EvalSymlinks(dotGit)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("primary %q .git is neither directory nor regular file", primary)
	}
	raw, err := readGitDirPointer(dotGit)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(primaryReal, raw)
	}
	return filepath.EvalSymlinks(raw)
}

func readSmallFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxGitFileBytes+1))
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	if len(data) > maxGitFileBytes {
		return nil, fmt.Errorf("%q is unexpectedly large", path)
	}
	return data, nil
}

func readGitDirPointer(path string) (string, error) {
	data, err := readSmallFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimRight(string(data), "\r\n")
	raw, ok := strings.CutPrefix(line, "gitdir: ")
	if !ok || strings.ContainsAny(raw, "\r\n") {
		return "", fmt.Errorf("%q has invalid gitdir pointer", path)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%q has empty gitdir pointer", path)
	}
	return raw, nil
}

// trustedWorktreeGitDir verifies that worktree's .git is a regular
// gitfile pointing to a direct child of primary's .git/worktrees/ whose
// backlink matches worktree and whose commondir points back to primary's
// git directory. This stops a sandboxed agent from replacing .git with
// a directory carrying a malicious git config executed by the daemon.
func trustedWorktreeGitDir(primary, worktree string) (string, error) {
	wtReal, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}
	dotGit := filepath.Join(wtReal, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return "", fmt.Errorf("stat worktree .git: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("worktree %q .git is not a regular file", worktree)
	}
	raw, err := readGitDirPointer(dotGit)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(wtReal, raw)
	}
	gitDir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", fmt.Errorf("resolve worktree gitdir: %w", err)
	}
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("worktree gitdir %q is not a directory", gitDir)
	}
	pGitDir, err := primaryGitDir(primary)
	if err != nil {
		return "", err
	}
	wtAdminBase, err := filepath.EvalSymlinks(filepath.Join(pGitDir, "worktrees"))
	if err != nil {
		return "", fmt.Errorf("resolve primary worktrees dir: %w", err)
	}
	if filepath.Dir(gitDir) != wtAdminBase {
		return "", fmt.Errorf("worktree gitdir %q is not a direct child of %q", gitDir, wtAdminBase)
	}
	commonRaw, err := readSmallFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return "", fmt.Errorf("read worktree commondir: %w", err)
	}
	common := strings.TrimSpace(string(commonRaw))
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	commonReal, err := filepath.EvalSymlinks(common)
	if err != nil || commonReal != pGitDir {
		return "", fmt.Errorf("worktree commondir %q does not match primary %q", common, pGitDir)
	}
	backlink, err := readSmallFile(filepath.Join(gitDir, "gitdir"))
	if err != nil {
		return "", fmt.Errorf("read worktree backlink: %w", err)
	}
	back := strings.TrimSpace(string(backlink))
	if !filepath.IsAbs(back) {
		back = filepath.Join(gitDir, back)
	}
	backDir, err := filepath.EvalSymlinks(filepath.Dir(back))
	if err != nil || filepath.Base(back) != ".git" || backDir != wtReal {
		return "", fmt.Errorf("worktree gitdir %q does not belong to %q", gitDir, worktree)
	}
	return gitDir, nil
}

func unsaved(ctx context.Context, r track.Repo, detached bool) (Unsaved, error) {
	u := Unsaved{Repo: r.Name}
	gitDir, err := trustedWorktreeGitDir(r.Path, r.Worktree)
	if err != nil {
		return u, err
	}
	wt := git.NewSafeWorktreeClient(gitDir, r.Worktree)
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
		gitDir, err := trustedWorktreeGitDir(r.Path, r.Worktree)
		if err != nil {
			continue
		}
		if b, err := git.NewSafeWorktreeClient(gitDir, r.Worktree).CurrentBranch(ctx); err == nil && b != "" {
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
		if _, err := os.Lstat(r.Worktree); errors.Is(err, os.ErrNotExist) {
			_ = primary.PruneWorktrees(ctx)
			continue
		}
		if w.Root != "" && !withinRoot(w.Root, r.Worktree) {
			errs = append(errs, fmt.Errorf("worktree %q is outside %q: remove it by hand or check XDG_STATE_HOME, then try again", r.Worktree, w.Root))
			continue
		}
		if err := primary.RemoveWorktree(ctx, r.Worktree); err != nil {
			errs = append(errs, fmt.Errorf("remove the worktree for %s: %w", r.Name, err))
			continue
		}
		if dir, err := trackDir(w.Root, id); err == nil {
			removeEmptyParents(filepath.Dir(r.Worktree), dir)
		}
	}
	// Only an empty folder goes: whatever is left in it isn't ours to
	// delete.
	if dir, err := trackDir(w.Root, id); err == nil {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
			if err := os.Remove(dir); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
