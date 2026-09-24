package tracksview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

const (
	maxCardContent = 72
	swatchNameLen  = 15
	swatchGap      = 2
)

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	header := m.header()
	hints := m.hints()
	bodyHeight := max(0, m.height-lipgloss.Height(header)-lipgloss.Height(hints))
	body := lipgloss.Place(m.width, bodyHeight, lipgloss.Center, lipgloss.Center, m.card())
	return lipgloss.JoinVertical(lipgloss.Left, header, body, hints)
}

func (m Model) fg(token theme.Token) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.palette.Color(token))
}

func (m Model) header() string {
	title := m.fg(theme.Accent).Bold(true).Render("Tracks")
	build := m.fg(theme.TextFaint).Render("v2 dev build · " + m.version)
	gap := max(1, m.width-2-lipgloss.Width(title)-lipgloss.Width(build))
	line := " " + title + strings.Repeat(" ", gap) + build + " "
	rule := m.fg(theme.BorderDefault).Render(strings.Repeat("─", m.width))
	return line + "\n" + rule
}

func (m Model) card() string {
	content := min(maxCardContent, max(20, m.width-8))
	title := m.fg(theme.TextDefault).Bold(true).Render("The Tracks window")
	about := m.fg(theme.TextMuted).Width(content).Render(
		"Your tracks will live here. Chunk 2 adds the track list, track windows and the footer; " +
			"chunk 3 adds this window's header and tabs.")
	variant := "light"
	if m.palette.Dark() {
		variant = "dark"
	}
	themeLine := m.fg(theme.TextFaint).Render(fmt.Sprintf("Theme: %s (%s background)", m.palette.Theme().Name, variant))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.palette.Color(theme.BorderDefault)).
		Padding(1, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", about, "", themeLine, "", m.swatches(content)))
}

// swatches shows every token as a coloured block with its name.
func (m Model) swatches(width int) string {
	cell := 2 + 1 + swatchNameLen + swatchGap
	cols := max(1, width/cell)
	var rows []string
	var row []string
	for i, token := range theme.All {
		block := lipgloss.NewStyle().Background(m.palette.Color(token)).Render("  ")
		name := m.fg(theme.TextMuted).Width(swatchNameLen + swatchGap).Render(string(token))
		row = append(row, block+" "+name)
		if len(row) == cols || i == len(theme.All)-1 {
			rows = append(rows, strings.Join(row, ""))
			row = nil
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) hints() string {
	key := m.fg(theme.TextAccent)
	text := m.fg(theme.TextFaint)
	return " " + key.Render("Ctrl+b d") + text.Render(" detach    ") +
		key.Render("./tracks --new-app stop") + text.Render(" stop Tracks v2")
}
