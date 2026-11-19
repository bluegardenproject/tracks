package review

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestTruncateOnce(t *testing.T) {
	big := strings.Repeat("y", maxDiffBytes+10)
	once := Truncate(big)
	if len(once) >= len(big)+200 || !strings.HasSuffix(once, truncatedEnd) {
		t.Fatalf("an over-cap diff wasn't cut with a note")
	}
	if Truncate(once) != once {
		t.Error("a diff cut before, as the CLI does, was cut again")
	}
	if Truncate("small") != "small" {
		t.Error("a small diff changed")
	}
}

func TestBrief(t *testing.T) {
	b := Brief("  You are a reviewer.\n", 8, "+changed\n")
	for _, want := range []string{"You are a reviewer.\n\n## In this session", "read-only mode", "Never edit a file, commit or push.",
		"`REVIEW OUTCOME:` line", "take no new tasks", "Candor level: 8/10", "## The diff\n\n+changed\n"} {
		if !strings.Contains(b, want) {
			t.Errorf("the brief lacks %q:\n%s", want, b)
		}
	}
	if strings.Contains(Brief("x", 0, "d"), "Candor level") {
		t.Error("an unset candor should not invent a level")
	}
	if !strings.Contains(Prompt("/r/brief.md"), "/r/brief.md") {
		t.Error("the prompt doesn't say where the brief is")
	}
}

// The brief is the reviewer's prompt, checked against a golden file:
// changing it is a decision of its own.
func TestBriefGolden(t *testing.T) {
	golden.Check(t, "testdata/brief.golden", Brief("<the reviewer instructions>", 3, "<the diff>")+"---\n"+Prompt("/data/hooks/t1/review/brief.md")+"\n")
}
