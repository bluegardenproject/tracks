package tracksview

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
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

// bannerBlock is the banner between two blank lines.
func (m Model) bannerBlock() []string {
	lines := []string{strings.Repeat(" ", m.width)}
	for _, b := range m.banner() {
		lines = append(lines, pad("  "+b, m.width))
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
	case tabProxy:
		return m.proxyView(width, height)
	case tabRepositories:
		return m.reposView(width, height)
	case tabEngines:
		return m.enginesView(width, height)
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
	if n := m.noticeOf(m.tab); n != nil && n.text != "" {
		return m.noticeView(*n)
	}
	var keys []keyHelp
	s := m.settings
	switch {
	case m.tab == tabStation && m.station.asking != nil:
		return "  " + joinKeys(key, text, questionKeys(*m.station.asking))
	case m.tab == tabStation && m.station.offline:
		return "  " + text.Render("Reconnecting to the daemon…")
	case m.tab == tabStation && len(m.station.tracks) > 0:
		keys = stationKeys
		if t, _ := m.selectedTrack(); t.Draft != nil {
			keys = draftKeys
		} else if t.Archived {
			keys = archivedKeys
		} else if t.Status == track.Closed {
			keys = selectKeys
		} else if !t.Open() {
			keys = endedStationKeys
		}
		if m.station.filter.On() {
			keys = slices.Concat(keys, clearFilterKeys)
		}
	case m.tab == tabStation && m.station.err == nil && m.station.filter.On():
		keys = clearFilterKeys
	case m.tab == tabStation && m.station.err == nil:
		keys = emptyStationKeys
	case m.picker != nil && m.pickerFor == pickProxy:
		return "  " + joinKeys(key, text, pickerKeys)
	case m.tab == tabProxy && m.proxy.adding:
		return "  " + joinKeys(key, text, proxyAddKeys)
	case m.tab == tabProxy:
		keys = proxyKeys
	case m.tab == tabRepositories && m.repos.editing:
		return "  " + joinKeys(key, text, repoFormKeys)
	case m.tab == tabRepositories:
		keys = repoListKeys
	case m.picker != nil && (m.pickerFor == pickModel || m.pickerFor == pickTypeModel):
		return "  " + joinKeys(key, text, modelPickerKeys)
	case m.picker != nil:
		return "  " + joinKeys(key, text, pickerKeys)
	case m.tab == tabEngines && m.engines.editing:
		return "  " + joinKeys(key, text, engineKeys)
	case m.tab == tabEngines:
		keys = engineListKeys
	case m.tab == tabSettings && s.editing:
		switch s.section {
		case sectionGeneral:
			return "  " + joinKeys(key, text, generalKeys)
		case sectionCreator:
			return "  " + joinKeys(key, text, creatorKeys)
		case sectionTracks:
			return "  " + joinKeys(key, text, typeKeys)
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
