package cursor

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

// TestRuleMatchesGolden keeps the global Cursor rule in testdata.
func TestRuleMatchesGolden(t *testing.T) {
	golden.Check(t, "testdata/rule.golden", rule)
}
