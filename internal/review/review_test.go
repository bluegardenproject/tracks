package review

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestBrief(t *testing.T) {
	b := Brief("  You are a reviewer.\n", 8, Packet{Diffs: []FileDiff{{Path: "a", Text: "+changed\n"}}})
	for _, want := range []string{"You are a reviewer.\n\n## In this session", "read-only mode", "Never edit a file, commit or push.",
		"`REVIEW OUTCOME:` line", "take no new tasks", "Candor level: 8/10", "## The diff\n\n+changed\n"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief lacks %q:\n%s", want, b)
		}
	}
	if b := Brief("x", 0, Packet{}); strings.Contains(b, "Candor level") || strings.Contains(b, "## Asked") || strings.Contains(b, "## The commits") {
		t.Error("an empty packet and unset candor should not invent a level, a request or commits")
	}
	if !strings.Contains(Prompt("/r/brief.md"), "/r/brief.md") {
		t.Error("the prompt doesn't say where the brief is")
	}
}

// The brief is the reviewer's prompt, checked against a golden file:
// changing it is a decision of its own.
func TestBriefGolden(t *testing.T) {
	p := Packet{
		Base:      "origin/main",
		Brief:     "<what the user asks of this review>",
		Commits:   "<commit messages>",
		Files:     []File{{Path: "a.go", Added: 3, Removed: 1}, {Path: "big.go", Added: 9000}, {Path: "logo.png", Binary: true}},
		Diffs:     []FileDiff{{Path: "a.go", Text: "<a.go's diff>\n"}, {Path: "a.go", Text: "<a.go's uncommitted diff>\n", Uncommitted: true}},
		Left:      []string{"big.go"},
		MoreFiles: 2,
	}
	golden.Check(t, "testdata/brief.golden", Brief("<the reviewer instructions>", 3, p)+"---\n"+Prompt("/data/hooks/t1/review/brief.md")+"\n")
}
