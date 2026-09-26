package repos

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Git is what repos needs to know about a checkout.
type Git interface {
	Checkout(ctx context.Context, dir string) (Checkout, error)
	DefaultBranch(ctx context.Context, dir string) (string, error)
	BranchExists(ctx context.Context, dir, branch string) (bool, error)
	Remote(ctx context.Context, dir string) (string, error)
}

// Checkout describes the git checkout a folder is in.
type Checkout struct {
	Top      string // its top folder, symlinks resolved
	Worktree bool   // a linked worktree rather than the main checkout
}

// Exec runs the git command line.
type Exec struct{}

func (Exec) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// localEnv point git at one repository (`git rev-parse
// --local-env-vars`). Hooks and `git rebase --exec` set them, and they
// win over -C.
var localEnv = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE", "GIT_GRAFT_FILE",
	"GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX", "GIT_SHALLOW_FILE",
	"GIT_COMMON_DIR",
}

// gitEnv is the environment without localEnv.
func gitEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(localEnv, name)
	})
}

// Checkout fails when dir isn't in a git checkout.
func (g Exec) Checkout(ctx context.Context, dir string) (Checkout, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return Checkout{}, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		return Checkout{}, errors.New("unexpected git rev-parse output")
	}
	top, err := filepath.EvalSymlinks(lines[0])
	if err != nil {
		return Checkout{}, err
	}
	return Checkout{Top: top, Worktree: filepath.Clean(lines[1]) != filepath.Clean(lines[2])}, nil
}

// DefaultBranch is the branch origin/HEAD points to, or else the
// checked-out one.
func (g Exec) DefaultBranch(ctx context.Context, dir string) (string, error) {
	if out, err := g.run(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(out, "origin/"), nil
	}
	return g.run(ctx, dir, "branch", "--show-current")
}

// BranchExists looks for branch locally and on origin.
func (g Exec) BranchExists(ctx context.Context, dir, branch string) (bool, error) {
	if strings.HasPrefix(branch, "-") {
		return false, nil
	}
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		_, err := g.run(ctx, dir, "rev-parse", "--verify", "--quiet", ref)
		if err == nil {
			return true, nil
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return false, err
		}
	}
	return false, nil
}

// Remote is origin's URL, "" without one.
func (g Exec) Remote(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", nil
	}
	return out, nil
}
