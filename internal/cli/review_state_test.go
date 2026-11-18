package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeStateSeesContentChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "one")
	if out, err := exec.Command("git", "-C", dir, "commit", "-q", "--allow-empty", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "a.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	write("b c.txt", "new")

	before, err := worktreeState(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ name, text string }{{"a.go", "two"}, {"b c.txt", "newer"}} {
		write(change.name, change.text)
		after, err := worktreeState(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if after == before {
			t.Errorf("editing %s, already dirty, went unnoticed", change.name)
		}
		before = after
	}
	if again, _ := worktreeState(ctx, dir); again != before {
		t.Error("the state changed with nothing changed")
	}
}
