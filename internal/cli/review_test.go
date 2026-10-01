package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

// repo is a git repo with a base commit, a commit on top and an edit
// that isn't committed.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "T")
	write("one\n")
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	run("update-ref", "refs/remotes/origin/develop", "HEAD")
	write("one\ncommitted\n")
	run("commit", "-q", "-am", "work")
	write("one\ncommitted\nuncommitted\n")
	return dir
}

func TestReviewBaseIsTheTrackRepos(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	tr := track.Track{Repos: []track.Repo{
		{Name: "other", Worktree: t.TempDir(), Base: "main"},
		{Name: "tracks", Worktree: dir, Base: "release"},
	}}
	if got := reviewBase(ctx, filepath.Join(dir, "."), tr); got != "origin/release" {
		t.Errorf("base %q, want the repo's own", got)
	}
	if got := reviewBase(ctx, dir, track.Track{}); got != "origin/develop" {
		t.Errorf("without the track, base %q, want the origin ref that exists", got)
	}
	if got := fallbackBase(ctx, t.TempDir()); got != "HEAD~1" {
		t.Errorf("outside a repo with origin refs, base %q", got)
	}
}

func TestBranchDiffIncludesUncommittedWork(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	got, err := branchDiff(ctx, dir, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "+committed") || !strings.Contains(got, "--- uncommitted changes") || !strings.Contains(got, "+uncommitted") {
		t.Errorf("the diff lacks the committed or uncommitted change:\n%s", got)
	}

	if err := exec.Command("git", "-C", dir, "commit", "-qam", "more").Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := branchDiff(ctx, dir, "HEAD"); err == nil || err.Error() != "no changes against HEAD: nothing to review" {
		t.Errorf("an empty diff: %v", err)
	}
	if _, err := branchDiff(ctx, dir, "origin/nope"); err == nil || !strings.Contains(err.Error(), "origin/nope") {
		t.Errorf("a base that doesn't exist: %v", err)
	}
}
