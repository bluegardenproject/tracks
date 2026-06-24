package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup creates a primary dir with a couple of gitignored files and an
// empty worktree dir, returning both paths.
func setup(t *testing.T) (primary, worktree string) {
	t.Helper()
	primary = t.TempDir()
	worktree = t.TempDir()
	if err := os.WriteFile(filepath.Join(primary, ".env"), []byte("TOP=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(primary, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, "a", "b.env"), []byte("NESTED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return primary, worktree
}

func collect() (func(string), *[]string) {
	var lines []string
	return func(s string) { lines = append(lines, s) }, &lines
}

func TestCopySymlink(t *testing.T) {
	primary, worktree := setup(t)
	emit, _ := collect()
	err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		CopyIgnored:  []string{".env", "a/*.env"},
		CopyMode:     "symlink",
	}, emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	link := filepath.Join(worktree, ".env")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat .env: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".env is not a symlink: %v", fi.Mode())
	}
	target, _ := os.Readlink(link)
	if target != filepath.Join(primary, ".env") {
		t.Errorf("symlink target = %q, want primary .env", target)
	}
	// Nested glob match should also be reproduced.
	if _, err := os.Lstat(filepath.Join(worktree, "a", "b.env")); err != nil {
		t.Errorf("nested a/b.env not provisioned: %v", err)
	}
}

func TestCopyMode(t *testing.T) {
	primary, worktree := setup(t)
	emit, _ := collect()
	if err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		CopyIgnored:  []string{".env"},
		CopyMode:     "copy",
	}, emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	dst := filepath.Join(worktree, ".env")
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error(".env should be a copy, not a symlink")
	}
	// Independent copy: changing the primary must not affect the worktree.
	if err := os.WriteFile(filepath.Join(primary, ".env"), []byte("CHANGED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "TOP=1\n" {
		t.Errorf("copy not independent: %q", got)
	}
}

func TestDepsCmdRuns(t *testing.T) {
	primary, worktree := setup(t)
	emit, lines := collect()
	err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		DepsCmd:      "echo hello && touch installed.marker",
	}, emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "installed.marker")); err != nil {
		t.Errorf("deps command did not run in worktree: %v", err)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "hello") {
		t.Errorf("deps output not streamed to emit: %v", *lines)
	}
}

func TestMissingGlobIsWarnedNotFatal(t *testing.T) {
	primary, worktree := setup(t)
	emit, lines := collect()
	err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		CopyIgnored:  []string{"does-not-exist.env"},
	}, emit)
	if err != nil {
		t.Fatalf("missing glob should not be fatal: %v", err)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "skipping") {
		t.Errorf("expected a skip warning, got: %v", *lines)
	}
}

func TestDepsCmdFailurePropagates(t *testing.T) {
	primary, worktree := setup(t)
	emit, _ := collect()
	err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		DepsCmd:      "exit 3",
	}, emit)
	if err == nil {
		t.Fatal("expected error from failing deps command")
	}
}
