package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// clearFilterLabel is the Filtered line's button, after its key.
const clearFilterLabel = "Clear filter"

// filterRows is how many rows the Filtered line takes above the table.
func (m Model) filterRows() int {
	if m.station.filter.On() {
		return 1
	}
	return 0
}

// filterLine says which filter Station is under, with Clear filter at
// the right, width cells wide.
func (m Model) filterLine(width int) string {
	button := m.fg(theme.TextAccent).Render("x") + " " + m.fg(theme.TextDefault).Underline(m.station.hoverClear).Render(clearFilterLabel)
	bw := lipgloss.Width(button)
	text := cut("Filtered: "+m.station.filter.String(), max(0, width-bw-2))
	gap := max(1, width-lipgloss.Width(text)-bw)
	return m.fg(theme.StateInfoText).Render(text) + strings.Repeat(" ", gap) + button
}

// onClearFilter reports whether cell x, y of the window is on Clear
// filter.
func (m Model) onClearFilter(x, y int) bool {
	if m.tab != tabStation || !m.station.filter.On() || m.station.err != nil {
		return false
	}
	p := m.panes()
	right := p.listX + p.listWidth - 2
	return y == m.contentTop()+1 && x < right && x >= right-len("x "+clearFilterLabel)
}

// clearFilter takes Station out of its filter.
func (m Model) clearFilter() tea.Cmd {
	if m.setFilter == nil {
		return nil
	}
	set := m.setFilter
	return func() tea.Msg {
		if err := set(track.Filter{}); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{ok: "Cleared the filter.", reload: true}
	}
}

// noMatch is the empty list's text under a filter.
func (m Model) noMatch(lines []string, width, height int) []string {
	text := m.fg(theme.TextMuted).Render("No tracks match the filter.")
	return append(lines, m.message(width, max(0, height-len(lines)), text)...)
}
