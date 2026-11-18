package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeStateSeesContentChanges(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
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
	for link, target := range map[string]string{"node_modules": t.TempDir(), "dangling": filepath.Join(dir, "nowhere")} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}

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
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if fromSub, _ := worktreeState(ctx, filepath.Join(dir, "sub")); fromSub != before {
		t.Error("the state differs when taken from a subdirectory")
	}
	if again, _ := worktreeState(ctx, dir); again != before {
		t.Error("the state changed with nothing changed")
	}
}

func TestTrackEngine(t *testing.T) {
	t.Setenv("CLAUDECODE", "")
	if got := trackEngine("claude"); got != "claude" {
		t.Errorf("a known engine became %s", got)
	}
	if got := trackEngine(""); got != "cursor" {
		t.Errorf("outside Claude Code the fallback is %s, want cursor", got)
	}
	t.Setenv("CLAUDECODE", "1")
	if got := trackEngine(""); got != "claude" {
		t.Errorf("inside Claude Code the fallback is %s, want claude", got)
	}
}
