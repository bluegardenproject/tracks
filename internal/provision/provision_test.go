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

func TestApfsCloneSeedsNodeModules(t *testing.T) {
	primary, worktree := setup(t)
	// Give the primary a node_modules with a sentinel file.
	if err := os.MkdirAll(filepath.Join(primary, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, "node_modules", "pkg", "index.js"), []byte("//\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emit, _ := collect()
	// DepsCmd reconciles after the seed; assert it runs in the worktree too.
	if err := Run(context.Background(), Options{
		PrimaryPath:   primary,
		WorktreePath:  worktree,
		CacheStrategy: "apfs-clone",
		DepsCmd:       "touch reconciled.marker",
	}, emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// node_modules was seeded (clone on APFS, plain copy elsewhere).
	if _, err := os.Stat(filepath.Join(worktree, "node_modules", "pkg", "index.js")); err != nil {
		t.Errorf("node_modules not seeded into worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "reconciled.marker")); err != nil {
		t.Errorf("deps reconcile did not run after clone: %v", err)
	}
}

func TestApfsCloneNoPrimaryNodeModulesIsNoop(t *testing.T) {
	primary, worktree := setup(t) // setup creates no node_modules
	emit, lines := collect()
	if err := Run(context.Background(), Options{
		PrimaryPath:   primary,
		WorktreePath:  worktree,
		CacheStrategy: "apfs-clone",
	}, emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "node_modules")); !os.IsNotExist(err) {
		t.Errorf("worktree node_modules should not exist when primary has none")
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "skipped") {
		t.Errorf("expected an apfs-clone skip notice, got: %v", *lines)
	}
}

func TestApfsCloneSkipsWhenWorktreeHasNodeModules(t *testing.T) {
	primary, worktree := setup(t)
	if err := os.MkdirAll(filepath.Join(primary, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Worktree already has its own node_modules — the clone must not clobber it.
	if err := os.MkdirAll(filepath.Join(worktree, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "node_modules", "OWN"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	emit, lines := collect()
	if err := Run(context.Background(), Options{
		PrimaryPath:   primary,
		WorktreePath:  worktree,
		CacheStrategy: "apfs-clone",
	}, emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "node_modules", "OWN")); err != nil {
		t.Errorf("existing worktree node_modules was clobbered: %v", err)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "skipped") {
		t.Errorf("expected an apfs-clone skip notice, got: %v", *lines)
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

func TestSkipDepsCmd(t *testing.T) {
	primary, worktree := setup(t)
	emit, _ := collect()
	err := Run(context.Background(), Options{
		PrimaryPath:  primary,
		WorktreePath: worktree,
		CopyIgnored:  []string{".env"},
		DepsCmd:      "touch should-not-exist.marker",
		SkipDepsCmd:  true,
	}, emit)
	if err != nil {
		t.Fatalf("Run with SkipDepsCmd: %v", err)
	}
	// Env files must still be copied.
	if _, err := os.Stat(filepath.Join(worktree, ".env")); err != nil {
		t.Errorf("env file not copied despite SkipDepsCmd=true: %v", err)
	}
	// DepsCmd must NOT have run.
	if _, err := os.Stat(filepath.Join(worktree, "should-not-exist.marker")); err == nil {
		t.Error("DepsCmd ran despite SkipDepsCmd=true")
	}
}

func TestRunDepsOnly(t *testing.T) {
	_, worktree := setup(t)
	emit, lines := collect()
	err := RunDepsOnly(context.Background(), Options{
		WorktreePath: worktree,
		DepsCmd:      "echo deps-ran && touch deps.marker",
	}, emit)
	if err != nil {
		t.Fatalf("RunDepsOnly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "deps.marker")); err != nil {
		t.Errorf("DepsCmd did not run: %v", err)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "deps-ran") {
		t.Errorf("deps output not streamed: %v", *lines)
	}
}

func TestRunDepsOnlyNoopWhenEmpty(t *testing.T) {
	_, worktree := setup(t)
	emit, _ := collect()
	if err := RunDepsOnly(context.Background(), Options{WorktreePath: worktree}, emit); err != nil {
		t.Fatalf("RunDepsOnly with empty DepsCmd: %v", err)
	}
}
