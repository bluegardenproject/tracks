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
			wantAll: []string{
				"REVIEW OUTCOME: pass",
				"REVIEW OUTCOME: blocked",
				"## Candor level",
				"Candor level: 3/10",
			},
		},
		{
			name:     "tracks-docs-reviewer",
			body:     docsReviewerAgentTemplate,
			wantName: "name: tracks-docs-reviewer",
			wantAll: []string{
				"DOC REVIEW OUTCOME: ship | revise | rework",
				"## Strengths (keep)",
				"## Opinion",
				"## Findings",
				"## Claim check",
				"## Not checked",
				"confirmed",
				"contradicted",
				"unverified",
				"## Candor level",
				// Brief keys the agent must recognise. claude.docReviewBrief
				// already emits Candor / Opinion / Claim check; Goal, Skip
				// slides, and Web research are documented ahead of the
				// daemon wiring (a follow-up) so the agent honours them the
				// moment the brief starts sending them.
				"`Candor level: N/10`",
				"`Opinion section: ON|OFF`",
				"`Claim check: ON|OFF`",
				"`Skip slides: <list>`",
				"`Web research: ON|OFF`",
				"`Goal: <text>`",
				// The two multi-line inline-code spans are the only
				// backtick escapes in the template that a compile can't
				// catch — an unbalanced one would still build but render
				// wrong. Pin both across their embedded newline.
				"`Goal: convince shareholders to fund feature A` or `Goal: present\n  midterm numbers to the finance team`",
				"the brief's `Skip\n     slides` list names",
				// Distinctive lines from the two behaviours the pass added,
				// guarding their presence in the template.
				"### 1 · slide 12 (block)",
				"Simulate the audience",
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

// Candor is a delivery setting. An agent that read a high level as
// permission to drop or downgrade findings would turn the dial into a
// way of ordering a friendlier verdict, so the invariant is pinned in
// both templates rather than left to the wording of the scale.
func TestCandorIsDeliveryOnlyInBothAgents(t *testing.T) {
	for name, body := range map[string]string{
		"tracks-reviewer":      reviewerAgentTemplate,
		"tracks-docs-reviewer": docsReviewerAgentTemplate,
	} {
		if !strings.Contains(body, "**Candor changes wording only.**") {
			t.Errorf("%s: missing the candor delivery-only invariant", name)
		}
	}
}

// The optional sections have to be gated in the workflow, not only
// listed in the report skeleton: an agent that runs the verification
// lookups and then hides the table has ignored the point of switching
// claim check off.
func TestDocsReviewerGatesOptionalSections(t *testing.T) {
	for _, want := range []string{
		"*(Claim check ON only",
		"*(Opinion ON only.)*",
		"**When claim check is OFF**",
		"repo, GitHub, or Jira lookups",
		// Both switches can be off at once, so findings must come from a
		// step no switch gates — otherwise that review has no source.
		"*(Always — the section no switch turns off.)*",
	} {
		if !strings.Contains(docsReviewerAgentTemplate, want) {
			t.Errorf("docs reviewer template missing %q", want)
		}
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
