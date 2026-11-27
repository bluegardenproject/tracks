package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/git"
)

// noHooks keeps a plugin's repository from running anything while
// Tracks clones and reads it.
var noHooks = []string{"-c", "core.hooksPath=/dev/null", "-c", "submodule.recurse=false"}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	out, _, err := git.ExecRunner{Dir: dir}.Run(ctx, append(append([]string{}, noHooks...), args...)...)
	return strings.TrimSpace(out), err
}

// clone clones url into dir, an empty folder, without checking it out.
func clone(ctx context.Context, url, dir string) error {
	if _, err := run(ctx, "", "clone", "--quiet", "--no-checkout", "--", url, dir); err != nil {
		return fmt.Errorf("clone %s: %w", url, err)
	}
	return nil
}

// resolve finds the commit ref names in a fresh clone: a branch on
// origin, a tag or a commit; the remote's default branch for "".
func resolve(ctx context.Context, dir, ref string) (string, error) {
	candidates := []string{"HEAD"}
	if ref != "" {
		candidates = []string{"refs/remotes/origin/" + ref, "refs/tags/" + ref, ref}
	}
	for _, c := range candidates {
		if sha, err := run(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", c+"^{commit}"); err == nil {
			return sha, nil
		}
	}
	if ref == "" {
		return "", fmt.Errorf("the repository has no default branch; add @<branch>")
	}
	return "", fmt.Errorf("no branch, tag or commit %s in the repository", ref)
}

// checkout checks commit out in dir, detached.
func checkout(ctx context.Context, dir, commit string) error {
	_, err := run(ctx, dir, "checkout", "--quiet", "--detach", commit)
	return err
}

// compare fetches old from the installed clone oldDir into dir, then
// returns the manifest's diff and the files changed between the two.
func compare(ctx context.Context, dir, oldDir, old, next string) (manifestDiff string, changed []string, err error) {
	if _, err := run(ctx, dir, "fetch", "--quiet", "--no-tags", "--", oldDir, old); err != nil {
		return "", nil, err
	}
	diff, _, err := git.ExecRunner{Dir: dir}.Run(ctx, append(append([]string{}, noHooks...),
		"diff", "--no-color", "--no-ext-diff", "--no-textconv", old, next, "--", "plugin.yaml")...)
	if err != nil {
		return "", nil, err
	}
	names, _, err := git.ExecRunner{Dir: dir}.Run(ctx, append(append([]string{}, noHooks...),
		"diff", "--name-only", "-z", "--no-renames", old, next)...)
	if err != nil {
		return "", nil, err
	}
	for _, n := range strings.Split(names, "\x00") {
		if n != "" {
			changed = append(changed, n)
		}
	}
	return diff, changed, nil
}
