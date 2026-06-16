package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo creates a throwaway primary checkout in $TMPDIR with one
// commit on "develop", suitable for exercising PrimaryRepoClient
// methods against real git. Returns the primary's path. Skips the
// test if git isn't installed.
func setupRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()

	bash := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	bash("git", "init", "-q", "-b", "develop", ".")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bash("git", "add", "README.md")
	bash("git", "commit", "-q", "-m", "initial")

	// Pretend "origin" is the same repo — that's enough to make
	// `fetch origin develop` succeed without needing a real remote.
	bash("git", "remote", "add", "origin", dir)
	bash("git", "fetch", "origin", "develop", "--quiet")

	return dir
}

func TestPrimaryAddRemoveWorktreeKeepsBranch(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	headBefore, err := c.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	worktree := filepath.Join(t.TempDir(), "wt")
	if err := c.AddWorktree(ctx, worktree, "fix/example", "origin/develop"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// Primary HEAD must be unchanged.
	headAfter, _ := c.CurrentBranch(ctx)
	if headBefore != headAfter {
		t.Errorf("primary moved %q -> %q", headBefore, headAfter)
	}

	// Worktree must be on the new branch.
	wt := NewWorktreeClient(worktree)
	wtBranch, err := wt.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if wtBranch != "fix/example" {
		t.Errorf("worktree on %q, want fix/example", wtBranch)
	}

	// Remove worktree.
	if err := c.RemoveWorktree(ctx, worktree); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree dir still present: %v", err)
	}

	// Branch must still exist locally — that's the whole point.
	exists, err := c.BranchExists(ctx, "fix/example")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("branch was deleted alongside worktree (should be retained)")
	}
}

func TestPrimaryAddWorktreeDetached(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	// FETCH_HEAD points at origin/develop after setupRepo's fetch.
	worktree := filepath.Join(t.TempDir(), "review-wt")
	if err := c.AddWorktreeDetached(ctx, worktree, "FETCH_HEAD"); err != nil {
		t.Fatalf("AddWorktreeDetached: %v", err)
	}

	// A detached worktree reports no current branch.
	wt := NewWorktreeClient(worktree)
	branch, err := wt.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "" {
		t.Errorf("detached worktree on branch %q, want empty", branch)
	}

	// It still has the base commit checked out.
	if _, err := os.Stat(filepath.Join(worktree, "README.md")); err != nil {
		t.Errorf("expected checked-out file: %v", err)
	}

	// Missing args are rejected.
	if err := c.AddWorktreeDetached(ctx, "", "FETCH_HEAD"); err == nil {
		t.Error("AddWorktreeDetached(\"\", ...) should error")
	}
}

func TestPrimaryListWorktrees(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	wt1 := filepath.Join(t.TempDir(), "wt1")
	if err := c.AddWorktree(ctx, wt1, "fix/a", "origin/develop"); err != nil {
		t.Fatal(err)
	}
	wt2 := filepath.Join(t.TempDir(), "wt2")
	if err := c.AddWorktree(ctx, wt2, "fix/b", "origin/develop"); err != nil {
		t.Fatal(err)
	}

	list, err := c.ListWorktrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d worktrees, want 3 (primary + 2)", len(list))
	}
	// Find by branch name.
	gotBranches := map[string]bool{}
	for _, w := range list {
		gotBranches[w.Branch] = true
	}
	if !gotBranches["fix/a"] || !gotBranches["fix/b"] {
		t.Errorf("missing branches in list: %v", gotBranches)
	}
}

func TestPrimaryBranchExists(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	got, err := c.BranchExists(ctx, "develop")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("develop should exist")
	}
	got, err = c.BranchExists(ctx, "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("does-not-exist should not exist")
	}
}

func TestPrimaryRejectsEmptyArgs(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()
	if err := c.AddWorktree(ctx, "", "b", "s"); err == nil {
		t.Error("empty path should error")
	}
	if err := c.DeleteBranch(ctx, ""); err == nil {
		t.Error("empty branch name should error")
	}
}

func TestIsTransient(t *testing.T) {
	cases := map[string]bool{
		"git worktree add: index.lock":   true,
		"cannot lock ref refs/heads/foo": true,
		"fatal: Unable to create '/x'":   true,
		"some unrelated error":           false,
	}
	for msg, want := range cases {
		got := isTransient(errors.New(msg))
		if got != want {
			t.Errorf("isTransient(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestWorktreeHasCommitsBeyond(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := c.AddWorktree(ctx, wt, "fix/x", "origin/develop"); err != nil {
		t.Fatal(err)
	}
	wc := NewWorktreeClient(wt)
	has, err := wc.HasCommitsBeyond(ctx, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("fresh worktree should not have commits beyond base")
	}

	// Add a commit and re-check.
	if err := os.WriteFile(filepath.Join(wt, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "x.txt"},
		{"git", "commit", "-q", "-m", "x"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wt
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	has, err = wc.HasCommitsBeyond(ctx, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("after a commit, worktree should be ahead of base")
	}
}

func TestExitErrorOf(t *testing.T) {
	if ExitErrorOf(errors.New("plain")) != -1 {
		t.Error("plain error should give -1")
	}
	// Trigger a real exit error via /usr/bin/false (or built-in).
	cmd := exec.Command("sh", "-c", "exit 7")
	err := cmd.Run()
	if got := ExitErrorOf(err); got != 7 {
		t.Errorf("exit 7 → %d", got)
	}
	_ = strings.TrimSpace // silence unused if reorganized
}
