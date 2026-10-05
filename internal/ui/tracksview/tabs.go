package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// tab is one tab of the Tracks window.
type tab struct {
	title string
	about string
}

// Tab indexes, in display order.
const (
	tabStation = iota
	tabProxy
	tabRepositories
	tabEngines
	tabSettings
)

var tabs = []tab{
	tabStation:      {"Station", "Your tracks, with their status and actions."},
	tabProxy:        {"Proxy", "Fixed ports that forward to a track's dev server."},
	tabRepositories: {"Repositories", "The repositories tracks are created from."},
	tabEngines:      {"Engines", "The agent CLIs that tracks run."},
	tabSettings:     {"Settings", "The theme, keys and where Tracks keeps its files."},
}

// The tab row's geometry, in cells. Clicks are mapped with the same
// numbers.
const (
	tabRows  = 3
	tabsLeft = 2
	tabGap   = 1
)

// liveMark follows the Proxy tab's title while a port forwards.
const liveMark = " ●"

// tabWidth is a tab's box: its title, padding and border.
func (m Model) tabWidth(i int) int { return lipgloss.Width(m.tabTitle(i)) + 6 }

// tabTitle is tab i's title as drawn, the Proxy tab's mark included.
func (m Model) tabTitle(i int) string {
	if i == tabProxy && m.proxy.live() {
		return tabs[i].title + liveMark
	}
	return tabs[i].title
}

// tabAt returns the tab drawn at cell x, y.
func (m Model) tabAt(x, y int) (int, bool) {
	top := m.tabsTop()
	if y < top || y >= top+tabRows {
		return 0, false
	}
	left := tabsLeft
	for i := range tabs {
		if x >= left && x < left+m.tabWidth(i) {
			return i, true
		}
		left += m.tabWidth(i) + tabGap
	}
	return 0, false
}

// tabBar draws every tab in a box of its own, tabRows tall. The active
// one is filled.
func (m Model) tabBar(width int) []string {
	gap := strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", tabGap)+"\n", tabRows), "\n")
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
		title := t.title
		if m.tabTitle(i) != t.title {
			// The mark keeps the tab's background, so the box stays whole.
			mark := lipgloss.NewStyle().Foreground(m.palette.Color(theme.StateSuccessText))
			if i == m.tab {
				mark = mark.Background(m.palette.Color(theme.TabActiveBg))
			}
			title += mark.Render(liveMark)
		}
		boxes = append(boxes, s.Render(title))
	}
	lines := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, boxes...), "\n")
	for i := range lines {
		lines[i] = pad(strings.Repeat(" ", tabsLeft)+lines[i], width)
	}
	return lines
}

// pad fills s with spaces up to width, or cuts it there.
func pad(s string, width int) string {
	s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}
