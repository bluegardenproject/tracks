package claude

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestReviewerLine(t *testing.T) {
	got := string(ReviewerLine("claude", "opus", "/h/claude-settings.json", "/r/brief.md", "Follow your instructions.")) + "\n" +
		string(ReviewerLine("claude", "", "", "/r/brief.md", "Follow your instructions.")) + "\n"
	golden.Check(t, "testdata/reviewer-line.golden", got)
}
