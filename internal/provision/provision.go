// Package provision makes a freshly created worktree runnable.
//
// A bare `git worktree add` produces a source-only checkout: it has no
// installed dependencies and none of the gitignored files (.env and
// friends) that live solely in the primary checkout. Provisioning fixes
// both, in order: first it brings the configured gitignored files over
// from the primary, then it runs a dependency-install command. The env
// files are copied first because the install step may read them.
//
// The package is deliberately decoupled from internal/config: the
// daemon translates a config.Provision into Options.
package provision

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options describes one worktree's provisioning. The zero value is a
// no-op (Run returns nil).
type Options struct {
	// PrimaryPath is the primary checkout to copy gitignored files from.
	PrimaryPath string
	// WorktreePath is the freshly created worktree to provision.
	WorktreePath string
	// CopyIgnored lists paths/globs (relative to PrimaryPath) to bring in.
	CopyIgnored []string
	// CopyMode is "symlink" (default) or "copy".
	CopyMode string
	// DepsCmd is the shell command to install dependencies. Empty skips it.
	DepsCmd string
}

// Run provisions the worktree. emit receives human-readable progress
// lines; it is never nil-checked, so callers must pass a non-nil func.
func Run(ctx context.Context, o Options, emit func(string)) error {
	if err := copyIgnored(o, emit); err != nil {
		return err
	}
	if strings.TrimSpace(o.DepsCmd) != "" {
		if err := runDeps(ctx, o, emit); err != nil {
			return err
		}
	}
	return nil
}

// copyIgnored reproduces each configured gitignored file from the
// primary checkout into the worktree. A pattern that matches nothing is
// warned about, not fatal — a stray entry shouldn't fail the track.
func copyIgnored(o Options, emit func(string)) error {
	if len(o.CopyIgnored) == 0 {
		return nil
	}
	mode := o.CopyMode
	if mode == "" {
		mode = "symlink"
	}
	for _, pattern := range o.CopyIgnored {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(o.PrimaryPath, pattern))
		if err != nil {
			return fmt.Errorf("bad copy_ignored pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			emit(fmt.Sprintf("provision: skipping %q (no match in primary)", pattern))
			continue
		}
		for _, src := range matches {
			rel, err := filepath.Rel(o.PrimaryPath, src)
			if err != nil {
				return fmt.Errorf("relativizing %q: %w", src, err)
			}
			dst := filepath.Join(o.WorktreePath, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("mkdir for %q: %w", rel, err)
			}
			// Replace any existing entry (the worktree may already track
			// a file at this path) so the link/copy is authoritative.
			_ = os.Remove(dst)
			if err := place(src, dst, mode); err != nil {
				return fmt.Errorf("provision %q: %w", rel, err)
			}
			emit(fmt.Sprintf("provision: %s %s", mode, rel))
		}
	}
	return nil
}

// place creates dst from src using the given mode.
func place(src, dst, mode string) error {
	if mode == "copy" {
		return copyFile(src, dst)
	}
	return os.Symlink(src, dst)
}

// copyFile copies a regular file's contents and mode bits.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// runDeps runs the install command in the worktree via the user's login
// shell, so it inherits the same PATH an interactive shell has (node /
// pnpm provided by nvm/fnm, etc.) — mirroring how Claude is spawned.
func runDeps(ctx context.Context, o Options, emit func(string)) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	emit(fmt.Sprintf("provision: installing deps (%s)", o.DepsCmd))
	cmd := exec.CommandContext(ctx, shell, "-lc", o.DepsCmd)
	cmd.Dir = o.WorktreePath
	cmd.Env = os.Environ()

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // merge stderr into the same stream
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting deps command: %w", err)
	}
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		emit("[deps] " + scanner.Text())
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("deps command %q failed: %w", o.DepsCmd, err)
	}
	return nil
}
