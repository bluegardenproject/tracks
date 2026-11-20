package claude

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReviewerNames(t *testing.T) {
	if !regexp.MustCompile(`(?m)^name: tracks-docs-reviewer$`).MatchString(docsReviewerAgent) {
		t.Errorf("the docs reviewer has no plain name:\n%s", docsReviewerAgent[:200])
	}
}

func TestInstallHelpers(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "agents")
	mine := filepath.Join(dir, "tracks-docs-reviewer.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(dir, "tracks-reviewer.md")
	if err := os.WriteFile(ours, []byte("---\nx-tracks-managed: \"1\"\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("the user's own\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skipped, err := InstallHelpers(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0] != mine {
		t.Errorf("skipped = %v, want the user's file", skipped)
	}
	if _, err := os.Stat(ours); !os.IsNotExist(err) {
		t.Errorf("the code reviewer subagent of ours is still installed: %v", err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "the user's own\n" {
		t.Error("the user's file was overwritten")
	}

	if err := os.Remove(mine); err != nil {
		t.Fatal(err)
	}
	if skipped, err := InstallHelpers(home); err != nil || len(skipped) != 0 {
		t.Fatalf("second install: %v, %v", skipped, err)
	}
	if b, _ := os.ReadFile(mine); string(b) != docsReviewerAgent {
		t.Error("a missing reviewer was not written")
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "tracks-add-repo", "SKILL.md")); string(b) != addRepoSkill {
		t.Error("the add-repo skill was not written")
	}

	if err := os.WriteFile(ours, []byte("the user's own reviewer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallHelpers(home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(ours); string(b) != "the user's own reviewer\n" {
		t.Error("the user's own tracks-reviewer was removed")
	}
}

func TestReviewerInstructions(t *testing.T) {
	got := ReviewerInstructions()
	if strings.HasPrefix(got, "---") || strings.Contains(got, "x-tracks-managed") {
		t.Errorf("the frontmatter is left in:\n%s", got[:min(200, len(got))])
	}
	for _, want := range []string{"You are a code-review specialist.", "REVIEW OUTCOME: blocked", "## Candor level"} {
		if !strings.Contains(got, want) {
			t.Errorf("the instructions lack %q", want)
		}
	}
}
