package relay

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestPointer(t *testing.T) {
	r := Message{From: "reviewer-527192"}
	w := Message{From: "watch-000001"}
	d := Message{From: "detective-000002"}
	var lines []string
	for _, msgs := range [][]Message{{r}, {r, r, r}, {r, w, r}, {r, w, d}} {
		lines = append(lines, Pointer(msgs))
	}
	golden.Check(t, "testdata/pointer.golden", strings.Join(lines, "\n")+"\n")
	for _, l := range lines {
		if l[0] < '1' || l[0] > '9' {
			t.Errorf("%q doesn't start with a digit", l)
		}
	}
}
