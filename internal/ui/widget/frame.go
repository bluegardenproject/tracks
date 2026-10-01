package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
)

// Frame draws body in a rounded border titled title, w by h cells, in
// border's colour. Body lines start one cell in from the border and are
// padded or cut to the w-4 cells left; missing lines are blank.
func Frame(p style.Palette, title string, body []string, w, h int, border theme.Token) []string {
	if w < 6 || h < 2 {
		return nil
	}
	b := lipgloss.NewStyle().Foreground(p.Color(border))
	title = cutTo(title, w-6)
	lines := []string{b.Render("╭─ ") + lipgloss.NewStyle().Foreground(p.Color(theme.TextDefault)).Bold(true).Render(title) +
		b.Render(" "+strings.Repeat("─", max(0, w-5-lipgloss.Width(title)))+"╮")}
	inner := w - 4
	for i := range h - 2 {
		line := ""
		if i < len(body) {
			line = lipgloss.NewStyle().MaxWidth(inner).Render(body[i])
		}
		lines = append(lines, b.Render("│")+" "+padTo(line, inner)+" "+b.Render("│"))
	}
	return append(lines, b.Render("╰"+strings.Repeat("─", w-2)+"╯"))
}
