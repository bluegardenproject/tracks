package cursor

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

// TestRuleMatchesGolden keeps the Cursor rules in testdata.
func TestRuleMatchesGolden(t *testing.T) {
	golden.Check(t, "testdata/rule.golden", rule)
	golden.Check(t, "testdata/pr-comments.golden", prCommentsRule)
}
