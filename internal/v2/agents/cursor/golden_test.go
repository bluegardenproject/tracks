package cursor

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/golden"
)

// TestRuleMatchesGolden keeps the global Cursor rule in testdata.
func TestRuleMatchesGolden(t *testing.T) {
	golden.Check(t, "testdata/rule.golden", rule)
}
