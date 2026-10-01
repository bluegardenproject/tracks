package claude

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReviewerNames(t *testing.T) {
	for _, r := range []string{reviewerAgent, docsReviewerAgent} {
		if !regexp.MustCompile(`(?m)^name: tracks-(docs-)?reviewer$`).MatchString(r) {
			t.Errorf("reviewer has no plain name:\n%s", r[:200])
		}
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
	if b, _ := os.ReadFile(ours); string(b) != reviewerAgent {
		t.Error("an older reviewer of ours was not replaced")
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
	for in, want := range map[string]string{
		"---\nname: x\n---\nbody":            "body",
		"body":                               "body",
		"---\nname: x\n---\nbody\n---\nmore": "body\n---\nmore",
	} {
		if got := frontmatter.ReplaceAllString(in, ""); got != want {
			t.Errorf("%q stripped to %q, want %q", in, got, want)
		}
	}
}
