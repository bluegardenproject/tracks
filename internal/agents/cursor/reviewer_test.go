package cursor

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestReviewerScript(t *testing.T) {
	r := Reviewer{Program: "agent", Model: "gpt-5", Chat: "c1", Workspace: "/w", Prompt: "Read your brief.",
		Report: "/r/report.md", Reply: "'tracks' hook --engine cursor --track t1 --reviewer r1 --reply-file '/r/report.md'",
		Status: "Cursor (gpt-5) is reviewing the change. It's done when its report shows here.",
		Stream: "'tracks' review-stream"}
	script := r.Script()
	for _, forbidden := range []string{"--force", "--yolo", " -f "} {
		if strings.Contains(script, forbidden) {
			t.Errorf("the reviewer runs with %s: %s", forbidden, script)
		}
	}
	golden.Check(t, "testdata/reviewer-script.golden", script+"\n")
}
