package git

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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
//
// We default the runner to NoOptionalLocks=true because the
// supervisor (and the dashboard) hit this client every 2 seconds
// with read-only diff/status calls. Without it, git's implicit
// index-refresh side effect contends with Claude's own writes
// inside the same worktree (e.g. `git add` after editing a file)
// over `.git/worktrees/<name>/index.lock`.
func NewWorktreeClient(path string) *WorktreeClient {
	return &WorktreeClient{
		Path:   path,
		Runner: ExecRunner{Dir: path, NoOptionalLocks: true},
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

// ShortStat counts the files / insertions / deletions in the diff
// between base..HEAD and the working tree's uncommitted changes
// combined. Used by the dashboard's CHANGES column.
//
// Implemented via two calls:
//   - `git diff --shortstat <base>..HEAD` for committed changes
//   - `git diff --shortstat HEAD` for uncommitted on top
//
// The result is the sum. base is typically "origin/<base-branch>";
// when it can't be resolved we fall back to comparing against the
// merge-base of HEAD and any tracked upstream.
type ShortStat struct {
	Files      int
	Insertions int
	Deletions  int
}

// shortStatLine matches one of the formats `git diff --shortstat`
// emits: " 5 files changed, 120 insertions(+), 30 deletions(-)"
// (any of the three clauses may be absent — e.g. a pure deletion
// has no "insertions(+)" clause).
var shortStatLine = regexp.MustCompile(`(?:(\d+) files? changed)?(?:, (\d+) insertions?\(\+\))?(?:, (\d+) deletions?\(-\))?`)

// ShortStat computes the diff summary between base..HEAD plus
// uncommitted edits. Empty/zero on any error so callers can treat
// failures as "nothing to show yet".
//
// Best-effort cleanup of stale index.lock files runs first — if a
// previous git invocation was SIGKILLed (e.g. a 3-second context
// timeout) it can leave the lock behind, and that wedges any
// subsequent write from Claude until removed.
func (c *WorktreeClient) ShortStat(ctx context.Context, base string) ShortStat {
	c.CleanStaleIndexLock(ctx, 30*time.Second)
	committed := parseShortStat(c.runShortStat(ctx, base+"..HEAD"))
	uncommitted := parseShortStat(c.runShortStat(ctx, "HEAD"))
	return ShortStat{
		Files:      committed.Files + uncommitted.Files,
		Insertions: committed.Insertions + uncommitted.Insertions,
		Deletions:  committed.Deletions + uncommitted.Deletions,
	}
}

// CleanStaleIndexLock removes the worktree's index.lock file when
// its mtime is older than maxAge. Safe because a healthy git
// operation holds the lock for milliseconds; anything older is
// stale from a crashed/killed git process. No-op when the lock
// doesn't exist.
func (c *WorktreeClient) CleanStaleIndexLock(ctx context.Context, maxAge time.Duration) {
	gitDir, _, err := c.Runner.Run(ctx, "rev-parse", "--git-dir")
	if err != nil {
		return
	}
	lockPath := filepath.Join(c.Path, strings.TrimSpace(gitDir), "index.lock")
	info, err := os.Stat(lockPath)
	if err != nil {
		return
	}
	if time.Since(info.ModTime()) > maxAge {
		_ = os.Remove(lockPath)
	}
}

func (c *WorktreeClient) runShortStat(ctx context.Context, rev string) string {
	out, _, err := c.Runner.Run(ctx, "diff", "--shortstat", rev)
	if err != nil {
		return ""
	}
	return out
}

func parseShortStat(out string) ShortStat {
	out = strings.TrimSpace(out)
	if out == "" {
		return ShortStat{}
	}
	m := shortStatLine.FindStringSubmatch(out)
	if m == nil {
		return ShortStat{}
	}
	files, _ := strconv.Atoi(m[1])
	ins, _ := strconv.Atoi(m[2])
	del, _ := strconv.Atoi(m[3])
	return ShortStat{Files: files, Insertions: ins, Deletions: del}
}

// ChangedFiles returns the list of files touched between base..HEAD
// (committed) plus the working tree's uncommitted changes. Each
// entry is `<status>\t<path>` (e.g. "M\tfoo.go"). Used by the
// dashboard's info modal.
func (c *WorktreeClient) ChangedFiles(ctx context.Context, base string) ([]string, error) {
	committedOut, _, err := c.Runner.Run(ctx, "diff", "--name-status", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	uncommittedOut, _, err := c.Runner.Run(ctx, "diff", "--name-status", "HEAD")
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, line := range strings.Split(strings.TrimSpace(committedOut+"\n"+uncommittedOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// CommitLog returns the short-form commit log between base..HEAD.
// Each entry is "<sha7> <subject>".
func (c *WorktreeClient) CommitLog(ctx context.Context, base string) ([]string, error) {
	out, _, err := c.Runner.Run(ctx, "log", "--oneline", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
