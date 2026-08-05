package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// The agent templates are large strings written to disk verbatim, so
// the things Claude Code needs in order to discover and use them are
// worth pinning: valid frontmatter, the exact agent name the prompts
// invoke by string, and the verdict line callers grep for.
func TestAgentTemplatesAreWellFormed(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantName string
		wantAll  []string
	}{
		{
			name:     "tracks-reviewer",
			body:     reviewerAgentTemplate,
			wantName: "name: tracks-reviewer",
			wantAll:  []string{"REVIEW OUTCOME: pass", "REVIEW OUTCOME: blocked"},
		},
		{
			name:     "tracks-docs-reviewer",
			body:     docsReviewerAgentTemplate,
			wantName: "name: tracks-docs-reviewer",
			wantAll: []string{
				"DOC REVIEW OUTCOME: ship | revise | rework",
				"## Strengths (keep)",
				"## Findings",
				"## Claim check",
				"## Not checked",
				"confirmed",
				"contradicted",
				"unverified",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(tc.body, "---\n") {
				t.Error("template must open with YAML frontmatter")
			}
			if strings.Count(tc.body, "\n---\n") < 1 {
				t.Error("template must close its frontmatter block")
			}
			if !strings.Contains(tc.body, tc.wantName) {
				t.Errorf("template missing %q", tc.wantName)
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(tc.body, want) {
					t.Errorf("template missing %q", want)
				}
			}
		})
	}
}

// The docs reviewer deliberately omits the `tools:` allowlist that
// tracks-reviewer pins — an allowlist would exclude the Atlassian MCP
// tools, whose names embed a per-user server name we can't discover.
// Jira grounding is most of this agent's value, so if someone adds a
// `tools:` line here they need to have thought about that trade-off.
func TestDocsReviewerInheritsToolset(t *testing.T) {
	frontmatter, _, ok := strings.Cut(strings.TrimPrefix(docsReviewerAgentTemplate, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("could not isolate frontmatter")
	}
	if strings.Contains(frontmatter, "tools:") {
		t.Error("docs reviewer must not pin a tools allowlist — it would exclude MCP tools (see docsReviewerAgentTemplate)")
	}
}

func TestInstallGlobalHelpersWritesBothAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := NewServer(config.Default(), state.NewMemoryStore(), "test")
	if err := s.InstallGlobalHelpers(); err != nil {
		t.Fatalf("InstallGlobalHelpers: %v", err)
	}

	for _, name := range []string{"tracks-reviewer.md", "tracks-docs-reviewer.md"} {
		path := filepath.Join(home, ".claude", "agents", name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(b) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "tracks-add-repo.md")); err != nil {
		t.Errorf("add-repo skill not installed: %v", err)
	}
}
