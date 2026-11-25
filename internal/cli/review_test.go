package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
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

func TestGatherPacket(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	p, err := gatherPacket(ctx, dir, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Diffs) != 2 || p.Diffs[0].Uncommitted || !strings.Contains(p.Diffs[0].Text, "+committed") ||
		!p.Diffs[1].Uncommitted || !strings.Contains(p.Diffs[1].Text, "+uncommitted") {
		t.Errorf("diffs %+v; want the committed change, then the uncommitted one", p.Diffs)
	}
	if len(p.Files) != 1 || p.Files[0] != (review.File{Path: "a.txt", Added: 2}) {
		t.Errorf("files %+v; want a.txt with the lines of both", p.Files)
	}
	if p.Base != "origin/develop" || !strings.Contains(p.Commits, " work") || strings.Contains(p.Commits, " base") {
		t.Errorf("base %q, commits %q; want the branch's own commit", p.Base, p.Commits)
	}

	for _, args := range [][]string{{"mv", "a.txt", "café.txt"}, {"-c", "diff.noprefix=true", "-c", "diff.renames=true", "commit", "-qam", "more"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := exec.Command("git", "-C", dir, "config", "diff.noprefix", "true").Run(); err != nil {
		t.Fatal(err)
	}
	p, err = gatherPacket(ctx, dir, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	var files, diffs []string
	for _, f := range p.Files {
		files = append(files, f.Path)
	}
	for _, d := range p.Diffs {
		diffs = append(diffs, d.Path)
	}
	if !slices.Equal(files, []string{"a.txt", "café.txt"}) || !slices.Equal(diffs, files) {
		t.Errorf("a renamed file, with diff.noprefix set: files %q, diffs %q; want the same paths in both", files, diffs)
	}

	if _, err := gatherPacket(ctx, dir, "HEAD"); err == nil || err.Error() != "no changes against HEAD: nothing to review" {
		t.Errorf("no changes: %v", err)
	}
	if _, err := gatherPacket(ctx, dir, "origin/nope"); err == nil || !strings.Contains(err.Error(), "origin/nope") {
		t.Errorf("a base that doesn't exist: %v", err)
	}
}

func TestReadBrief(t *testing.T) {
	if got, err := readBrief(strings.NewReader("  from stdin\n"), "-"); err != nil || got != "from stdin" {
		t.Errorf("stdin: %q, %v", got, err)
	}
	if _, err := readBrief(nil, filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("a missing file was read")
	}
}

func TestOwnedPacket(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	p, err := gatherPacket(ctx, dir, "origin/develop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownedPacket(ctx, dir, p, "team"); err == nil || !strings.Contains(err.Error(), "no CODEOWNERS") {
		t.Errorf("without CODEOWNERS: %v; want the review stopped", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "CODEOWNERS"), []byte("*.txt @team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ownedPacket(ctx, dir, p, "team"); err == nil {
		t.Error("a CODEOWNERS that isn't committed was read: the base has none")
	}
	for _, args := range [][]string{{"add", ".github"}, {"commit", "-qm", "owners"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if _, err := ownedPacket(ctx, dir, p, "team"); err == nil {
		t.Error("the branch's own CODEOWNERS set its scope: GitHub reads the base's")
	}
	for _, args := range [][]string{{"update-ref", "refs/remotes/origin/develop", "HEAD"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	owned, err := ownedPacket(ctx, dir, p, "team")
	if err != nil || owned.Scope.Owned != 1 || len(owned.Diffs) != 2 {
		t.Errorf("%+v, %v; want @team's a.txt, committed and not", owned.Scope, err)
	}
	if _, err := ownedPacket(ctx, dir, p, "@someone"); err == nil || !strings.Contains(err.Error(), "owns none") {
		t.Errorf("an owner of nothing: %v", err)
	}
}

// --wait and --ask check the checkout of the reviewer they address.
func TestReviewerTarget(t *testing.T) {
	trains := []rpc.Train{
		{Handle: "reviewer-111111", Agent: "Reviewer", Target: "/w/api"},
		{Handle: "reviewer-222222", Agent: "Reviewer", Target: "/w/fix"},
		{Handle: "w-1", Agent: "Worker"},
	}
	for _, c := range []struct {
		trains []rpc.Train
		to     string
		want   string
	}{
		{trains, "reviewer-222222", "/w/fix"},
		{trains, "@reviewer-111111", "/w/api"},
		{trains, "reviewer", ""},
		{trains, "", ""},
		{trains[:1], "", "/w/api"},
		{trains[2:], "", ""},
	} {
		if got := reviewerTarget(c.trains, c.to); got != c.want {
			t.Errorf("reviewerTarget(%d trains, %q) = %q, want %q", len(c.trains), c.to, got, c.want)
		}
	}
}
