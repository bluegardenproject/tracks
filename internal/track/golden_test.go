package track

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

// labelCases are the window labels v1 gave, kept in testdata.
var labelCases = []struct{ name, document, prompt string }{
	{name: "Rate bug", prompt: "anything"},
	{name: "  ", prompt: "Investigate the rate spike on swap quotes since Monday"},
	{name: "fix: a.b c", prompt: "Investigate the rate spike on swap quotes since Monday"},
	{document: "/Users/u/Downloads/Q3 Architecture.deck.pdf", prompt: "Investigate the rate spike on swap quotes since Monday"},
	{name: "own name", document: "/tmp/x.md"},
	{prompt: "日本語だけ"},
}

func TestLabelsAndCandorMatchGolden(t *testing.T) {
	var out strings.Builder
	out.WriteString("=== Window labels\n")
	for _, c := range labelCases {
		fmt.Fprintf(&out, "%q %q %q -> %q\n", c.name, c.document, c.prompt, WindowLabel(c.name, c.document, c.prompt))
	}
	fmt.Fprintf(&out, "\n=== Candor %d to %d, default %d\n", MinCandor, MaxCandor, DefaultCandor)
	for level := MinCandor - 1; level <= MaxCandor+1; level++ {
		fmt.Fprintf(&out, "%d -> %d %s\n", level, CandorLevel(level), CandorLabel(level))
	}
	golden.Check(t, "testdata/labels.golden", out.String())
}
