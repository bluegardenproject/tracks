package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

const (
	// bannerBlock is the banner with a blank line above and below.
	bannerBlock = bannerRows + 2
	// minContent is how many content lines stay before the banner
	// gives way.
	minContent = 5
	// tabsChrome is the tabs with a blank line below.
	tabsChrome = tabRows + 1
)

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	var lines []string
	if m.showBanner() {
		lines = append(lines, m.bannerBlock()...)
	}
	lines = append(lines, m.tabBar(m.width)...)
	lines = append(lines, strings.Repeat(" ", m.width))
	lines = append(lines, m.content(m.width, m.contentHeight())...)
	lines = append(lines, pad(m.hints(), m.width))
	if len(lines) > m.height {
		lines = lines[len(lines)-m.height:]
	}
	return strings.Join(lines, "\n")
}

// showBanner reports whether the banner fits above the tabs: the hint
// row, the tabs and minContent lines of content come first.
func (m Model) showBanner() bool {
	return m.height-1-tabsChrome-bannerBlock >= minContent && m.width >= bannerWidth+4
}

// contentHeight is the height of the tab's content, between the tabs
// and the hint row.
func (m Model) contentHeight() int {
	h := m.height - 1 - tabsChrome
	if m.showBanner() {
		h -= bannerBlock
	}
	return max(0, h)
}

// tabsTop is the screen line the tabs start on.
func (m Model) tabsTop() int {
	if m.showBanner() {
		return bannerBlock
	}
	return 0
}

func (m Model) fg(token theme.Token) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.palette.Color(token))
}

// bannerBlock is the banner with the build on the right of its last
// lines.
func (m Model) bannerBlock() []string {
	right := make([]string, bannerRows)
	right[bannerRows-2] = m.fg(theme.TextFaint).Render("v2 dev build")
	right[bannerRows-1] = m.fg(theme.TextFaint).Render(m.version)
	lines := []string{strings.Repeat(" ", m.width)}
	for i, b := range m.banner() {
		left := "  " + b
		gap := m.width - lipgloss.Width(left) - lipgloss.Width(right[i]) - 2
		if gap < 2 {
			lines = append(lines, pad(left, m.width))
			continue
		}
		lines = append(lines, left+strings.Repeat(" ", gap)+right[i]+"  ")
	}
	return append(lines, strings.Repeat(" ", m.width))
}

// content is the active tab, width by height cells; tabs without
// content yet show a placeholder.
func (m Model) content(width, height int) []string {
	if height == 0 {
		return nil
	}
	switch m.tab {
	case tabStation:
		return m.stationView(width, height)
	case tabRepositories:
		return m.reposView(width, height)
	case tabSettings:
		return m.settingsView(width, height)
	}
	t := tabs[m.tab]
	block := lipgloss.JoinVertical(lipgloss.Left,
		m.fg(theme.TextDefault).Bold(true).Render(t.title),
		"",
		m.fg(theme.TextMuted).Render(t.about),
	)
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, block)
	lines := strings.Split(placed, "\n")
	for i := range lines {
		lines[i] = pad(lines[i], width)
	}
	return lines[:min(len(lines), height)]
}

// hints is the bottom row: the tab's notice, or the keys that work
// right now.
func (m Model) hints() string {
	key := m.fg(theme.TextAccent)
	text := m.fg(theme.TextFaint)
	var n notice
	switch m.tab {
	case tabStation:
		n = m.station.notice
	case tabRepositories:
		n = m.repos.notice
	case tabSettings:
		n = m.settings.notice
	}
	if n.text != "" {
		color := theme.StateSuccessText
		if n.err {
			color = theme.StateDangerText
		}
		return "  " + m.fg(color).Render(n.text)
	}
	var keys []keyHelp
	s := m.settings
	switch {
	case m.tab == tabStation && len(m.station.tracks) > 0:
		keys = stationKeys
	case m.tab == tabRepositories && m.repos.editing:
		return "  " + joinKeys(key, text, repoFormKeys)
	case m.tab == tabRepositories:
		keys = repoListKeys
	case s.picker != nil:
		return "  " + joinKeys(key, text, pickerKeys)
	case m.tab == tabSettings && s.editing:
		switch s.section {
		case sectionGeneral:
			return "  " + joinKeys(key, text, themeFieldKeys)
		case sectionCreator:
			return "  " + joinKeys(key, text, creatorKeys)
		}
		return "  " + joinKeys(key, text, scrollKeys)
	case m.tab == tabSettings && s.section != sectionFastTracks && s.section != sectionAbout:
		keys = settingsListKeys
	case m.tab == tabSettings:
		keys = settingsListKeys[:1]
	}
	keys = append(append([]keyHelp{}, keys...), tabKeys...)
	keys = append(keys, prefixKeys()[0])
	return "  " + joinKeys(key, text, keys)
}
