package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// tab is one tab of the Tracks window. Its content is a placeholder
// until chunk 7.
type tab struct {
	title string
	about string
}

// Tab indexes, in display order.
const (
	tabStation = iota
	tabRepositories
	tabProxy
	tabEngines
	tabSettings
)

var tabs = []tab{
	tabStation:      {"Station", "Your active tracks, with their status and actions."},
	tabRepositories: {"Repositories", "The repositories tracks are created from."},
	tabProxy:        {"Proxy", "Dev servers and the local proxy."},
	tabEngines:      {"Engines", "The agent CLIs that tracks run."},
	tabSettings:     {"Settings", "Tracks settings. For now: the theme."},
}

// tabRows is the height of the tab row.
const tabRows = 3

// tabBar draws every tab in a box of its own, tabRows tall. The active
// one is filled.
func (m Model) tabBar(width int) []string {
	gap := strings.Repeat(" \n", tabRows-1) + " "
	var boxes []string
	for i, t := range tabs {
		if i > 0 {
			boxes = append(boxes, gap)
		}
		s := lipgloss.NewStyle().Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.palette.Color(theme.TabBorder)).
			Foreground(m.palette.Color(theme.TabText))
		if i == m.tab {
			s = s.Bold(true).
				BorderForeground(m.palette.Color(theme.TabActiveBorder)).
				Foreground(m.palette.Color(theme.TabActiveText)).
				Background(m.palette.Color(theme.TabActiveBg))
		}
		boxes = append(boxes, s.Render(t.title))
	}
	lines := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, boxes...), "\n")
	for i := range lines {
		lines[i] = pad("  "+lines[i], width)
	}
	return lines
}

// pad fills s with spaces up to width, or cuts it there.
func pad(s string, width int) string {
	s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}
