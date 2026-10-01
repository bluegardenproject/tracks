package addtrack

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/golden"
)

// TestTextsMatchGolden keeps the types' descriptions, template prompts
// and the doc review's sections in testdata.
func TestTextsMatchGolden(t *testing.T) {
	var out strings.Builder
	for _, k := range kinds {
		fmt.Fprintf(&out, "=== %s\n%s\n\n%s\n\n", k.label, k.about, k.prompt)
	}
	out.WriteString("=== Sections\n" + strings.Join(sections, "\n") + "\n")
	golden.Check(t, "testdata/texts.golden", out.String())
}
