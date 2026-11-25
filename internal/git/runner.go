// Package git wraps the operations `tracks` needs against git
// repositories.
//
// The package deliberately exposes TWO clients with different
// capabilities:
//
//   - PrimaryRepoClient operates against the user's primary checkout
//     (the one Cursor watches). Its method set is strictly limited
//     to operations that DO NOT move the primary's HEAD: fetch,
//     worktree add/remove/list/prune, and branch existence queries.
//     Mutating the working tree or HEAD is unavailable by
//     construction.
//
//   - WorktreeClient operates inside a worktree `tracks` itself
//     created. Anything goes there — submodule init, status,
//     whatever. Cursor never touches that tree.
//
// The single architectural rule of `tracks` (see plan §"Cursor
// isolation invariant") is enforced by this type split.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner abstracts running a `git` invocation. Production uses
// ExecRunner; tests inject a fake that records arguments and returns
// canned output.
type Runner interface {
	Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
}

// ExecRunner shells out to the real `git` binary.
type ExecRunner struct {
	// Dir is the working directory to invoke git in. Empty string
	// means inherit from the parent process.
	Dir string

	// GitDir and WorkTree, when set, pass --git-dir / --work-tree
	// explicitly so git never discovers a tampered .git inside an
	// untrusted worktree.
	GitDir   string
	WorkTree string

	// Safe disables repository-controlled git config hooks and tools
	// (fsmonitor, hooksPath, sshCommand, diff.external, submodules)
	// when inspecting agent-writable worktrees from the daemon.
	Safe bool

	// NoOptionalLocks sets GIT_OPTIONAL_LOCKS=0 on the spawned
	// process so git skips lock-taking operations that exist only
	// as optimisations (the implicit index refresh during
	// `git status` / `git diff`). Use for read-only callers that
	// run concurrently with Claude (or another git process)
	// inside the same worktree, where the racing index.lock would
	// otherwise wedge writes.
	NoOptionalLocks bool
}

// command builds the *exec.Cmd that Run executes. Extracted so tests
// can assert argv/env/cwd without invoking git.
//
// `tracks` orchestrates git non-interactively, so we override
// GIT_EDITOR / EDITOR to a no-op. Without this, any git path that
// internally opens an editor (e.g. `commit` during a conflict, or
// `merge --no-ff` without -m) would fail with "Terminal is dumb,
// EDITOR unset" — not because the user did anything wrong, but
// because the daemon has no TTY.
func (e ExecRunner) command(ctx context.Context, args ...string) *exec.Cmd {
	var argv []string
	if e.Safe {
		argv = append(argv,
			"-c", "core.fsmonitor=false",
			"-c", "core.hooksPath=/dev/null",
			"-c", "core.sshCommand=false",
			"-c", "core.pager=cat",
			"-c", "diff.external=",
			"-c", "diff.ignoreSubmodules=all",
			"-c", "status.submoduleSummary=false",
			"-c", "submodule.recurse=false",
			"-c", "credential.helper=",
		)
	}
	if e.GitDir != "" {
		argv = append(argv, "--git-dir", e.GitDir)
	}
	if e.WorkTree != "" {
		argv = append(argv, "--work-tree", e.WorkTree)
	}
	if e.Safe && len(args) > 0 && args[0] == "status" {
		argv = append(argv, "status", "--ignore-submodules=all")
		argv = append(argv, args[1:]...)
	} else {
		argv = append(argv, args...)
	}
	cmd := exec.CommandContext(ctx, "git", argv...)
	if e.Dir != "" {
		cmd.Dir = e.Dir
	}
	env := append(os.Environ(), "GIT_EDITOR=:", "EDITOR=:")
	if e.NoOptionalLocks {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}
	cmd.Env = env
	return cmd
}

// Run executes `git args...` and returns stdout, stderr, and any
// non-zero exit error. It does not parse output — that's the
// caller's job.
func (e ExecRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	cmd := e.command(ctx, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			err = fmt.Errorf("git %s: %w: %s",
				strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
		} else {
			err = fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
	}
	return outBuf.String(), errBuf.String(), err
}

// ExitErrorOf returns the exit code from err if err wraps an
// *exec.ExitError; otherwise -1. Useful for callers that want to
// treat `git rev-parse` returning 128 as "no such ref" rather than
// a hard failure.
func ExitErrorOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
