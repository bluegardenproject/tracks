package claude

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/golden"
)

// TestHelpersMatchGolden keeps the files InstallHelpers writes in
// testdata.
func TestHelpersMatchGolden(t *testing.T) {
	for name, text := range map[string]string{
		"reviewer":      reviewerAgent,
		"docs-reviewer": docsReviewerAgent,
		"add-repo":      addRepoSkill,
	} {
		golden.Check(t, "testdata/"+name+".golden", text)
	}
}
