package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The reviewer prompt must come from the compiled template, not from a
// file on disk. Reading the installed copy would fail where the daemon
// has never run, and — since tracks will not overwrite a file it does
// not own — would silently use a user's own tracks-reviewer.md.
func TestReviewerInstructionsComeFromTheBinary(t *testing.T) {
	got, err := reviewerInstructions()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got, "---") || strings.Contains(got, "x-tracks-managed") {
		t.Errorf("frontmatter was not stripped:\n%s", got[:min(200, len(got))])
	}
	for _, want := range []string{"REVIEW OUTCOME", "block", "warn", "hint"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions are missing %q — is this the reviewer definition?", want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// The frontmatter stripper must not eat the body, and must cope with a
// document that has none.
func TestFrontmatterStripping(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		// The closing --- and its newline are consumed, so the body
		// starts immediately after them.
		"normal":    {"---\nname: x\n---\nbody here", "body here"},
		"no matter": {"body here", "body here"},
		// A --- later in the body must survive: the match is anchored
		// and non-greedy, so only the opening block goes.
		"body dashes": {"---\nname: x\n---\nbody\n---\nmore", "body\n---\nmore"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := frontmatter.ReplaceAllString(c.in, ""); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// fallbackBase only guesses when there is no configured base to read.
// The guess order matters less than the fact that it is a fallback:
// reviewing a develop-based repo against origin/main hands the reviewer
// every commit develop has that main doesn't.
func TestFallbackBasePrefersAnExistingRef(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "T")
	run("commit", "-q", "--allow-empty", "-m", "one")

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// No origin/* refs exist, so it must fall through to something that
	// resolves rather than naming a ref that doesn't exist.
	if got := fallbackBase(); got != "HEAD~1" {
		t.Errorf("fallbackBase = %q with no origin refs, want HEAD~1", got)
	}

	// Create origin/develop only; it must be found rather than skipped.
	run("update-ref", "refs/remotes/origin/develop", "HEAD")
	if got := fallbackBase(); got != "origin/develop" {
		t.Errorf("fallbackBase = %q, want origin/develop when it is the only origin ref", got)
	}
}

// The gate fires before a push, which is often before the last commit,
// so a review that saw only committed work would read stale content.
func TestBranchDiffIncludesUncommittedWork(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "work")
	// Now an edit that is NOT committed.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ncommitted\nuncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	got, err := branchDiff("HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "committed") {
		t.Error("diff is missing the committed change")
	}
	if !strings.Contains(got, "uncommitted") {
		t.Error("diff is missing uncommitted work — the gate fires before the last commit")
	}
}

// An empty diff is its own error, not an empty review.
func TestBranchDiffRefusesWhenThereIsNothing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"},
		{"config", "user.name", "T"}, {"commit", "-q", "--allow-empty", "-m", "one"},
		{"commit", "-q", "--allow-empty", "-m", "two"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := branchDiff("HEAD~1"); err == nil {
		t.Error("an empty diff should be an error rather than an empty review")
	}
}
