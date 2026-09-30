package claude

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// v1Consts evaluates the string constants of one v1 source file.
func v1Consts(t *testing.T, path string) map[string]string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Only the constants matter; the rest of the file doesn't resolve on
	// its own, and those errors are ignored.
	conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
	pkg, _ := conf.Check("v1", fset, []*ast.File{f}, nil)
	out := map[string]string{}
	for _, name := range pkg.Scope().Names() {
		if c, ok := pkg.Scope().Lookup(name).(*types.Const); ok && c.Val().Kind() == constant.String {
			out[name] = constant.StringVal(c.Val())
		}
	}
	return out
}

func TestReviewersMatchV1(t *testing.T) {
	v1 := v1Consts(t, "../../../daemon/skill.go")
	names := strings.NewReplacer("tracks-reviewer", "tracks-v2-reviewer", "tracks-docs-reviewer", "tracks-v2-docs-reviewer")
	for v2, want := range map[string]string{
		reviewerAgent:     v1["ReviewerAgentTemplate"],
		docsReviewerAgent: v1["docsReviewerAgentTemplate"],
	} {
		if want == "" {
			t.Fatal("v1 reviewer not found")
		}
		if v2 != names.Replace(want) {
			t.Errorf("reviewer differs from v1:\n%s", v2[:200])
		}
	}
	for _, r := range []string{reviewerAgent, docsReviewerAgent} {
		if !regexp.MustCompile(`(?m)^name: tracks-v2-(docs-)?reviewer$`).MatchString(r) {
			t.Errorf("reviewer has no v2 name:\n%s", r[:200])
		}
	}
}

func TestInstallHelpers(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "agents")
	mine := filepath.Join(dir, "tracks-v2-docs-reviewer.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(dir, "tracks-v2-reviewer.md")
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
	if b, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "tracks-v2-add-repo", "SKILL.md")); string(b) != addRepoSkill {
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
