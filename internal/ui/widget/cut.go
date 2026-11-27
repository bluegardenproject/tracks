package widget

import (
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Cut shortens s to at most width terminal cells, ending with an
// ellipsis when it doesn't fit.
func Cut(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(s, width, "…")
}

// CutLeft shortens s from the left to at most width terminal cells,
// starting with an ellipsis when it doesn't fit.
func CutLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	r := []rune(s)
	keep := len(r)
	for keep > 0 && lipgloss.Width(string(r[keep-1:])) <= width-1 {
		keep--
	}
	for keep < len(r) && (r[keep] == '\u200d' || unicode.Is(unicode.Mn, r[keep])) {
		keep++
	}
	return "…" + string(r[keep:])
}
