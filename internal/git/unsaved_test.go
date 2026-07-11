package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// commitInWorktree stages everything and commits in the worktree at
// path, so tests can create local-only (unpushed) history.
func commitInWorktree(t *testing.T, path, msg string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-q", "-m", msg},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = path
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

func TestUnsavedWorkCleanPushedWorktree(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	worktree := filepath.Join(t.TempDir(), "wt")
	// Started from origin/develop, so HEAD is on a remote ref: nothing
	// uncommitted, nothing unpushed.
	if err := c.AddWorktree(ctx, worktree, "fix/clean", "origin/develop"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	wt := NewWorktreeClient(worktree)
	reason, err := wt.UnsavedWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Errorf("clean+pushed worktree reported unsaved work: %q", reason)
	}
}

func TestUnsavedWorkDirtyWorktree(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	worktree := filepath.Join(t.TempDir(), "wt")
	if err := c.AddWorktree(ctx, worktree, "fix/dirty", "origin/develop"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	// An untracked file is uncommitted work that lives only here.
	if err := os.WriteFile(filepath.Join(worktree, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wt := NewWorktreeClient(worktree)
	reason, err := wt.UnsavedWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reason == "" {
		t.Fatal("dirty worktree reported as safe to delete")
	}
}

func TestUnsavedWorkUnpushedCommits(t *testing.T) {
	primary := setupRepo(t)
	c := NewPrimaryRepoClient(primary)
	ctx := context.Background()

	worktree := filepath.Join(t.TempDir(), "wt")
	if err := c.AddWorktree(ctx, worktree, "feat/unpushed", "origin/develop"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	// Commit locally without pushing: clean tree, but the commit exists
	// on no remote.
	if err := os.WriteFile(filepath.Join(worktree, "feature.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitInWorktree(t, worktree, "add feature")

	wt := NewWorktreeClient(worktree)

	unpushed, err := wt.UnpushedCommits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !unpushed {
		t.Error("local-only commit not detected as unpushed")
	}

	reason, err := wt.UnsavedWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reason == "" {
		t.Error("worktree with unpushed commit reported as safe to delete")
	}
}
