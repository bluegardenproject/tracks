package tracksview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

const (
	swatchNameLen = 18
	swatchGap     = 2
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
	content := m.height - 1 - tabsChrome
	if m.showBanner() {
		lines = append(lines, m.bannerBlock()...)
		content -= bannerBlock
	}
	lines = append(lines, m.tabBar(m.width)...)
	lines = append(lines, strings.Repeat(" ", m.width))
	lines = append(lines, m.content(m.width, max(0, content))...)
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

// content is the active tab's placeholder, width by height cells.
func (m Model) content(width, height int) []string {
	if height == 0 {
		return nil
	}
	t := tabs[m.tab]
	parts := []string{
		m.fg(theme.TextDefault).Bold(true).Render(t.title),
		"",
		m.fg(theme.TextMuted).Render(t.about),
	}
	if m.tab == tabSettings {
		variant := "light"
		if m.palette.Dark() {
			variant = "dark"
		}
		parts = append(parts, "",
			m.fg(theme.TextFaint).Render(fmt.Sprintf("Theme: %s (%s background)", m.palette.Theme().Name, variant)),
			"", m.swatches(min(width-4, 88)))
	}
	block := lipgloss.JoinVertical(lipgloss.Left, parts...)
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, block)
	lines := strings.Split(placed, "\n")
	for i := range lines {
		lines[i] = pad(lines[i], width)
	}
	return lines[:min(len(lines), height)]
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
	sep := text.Render(" · ")
	return "  " + key.Render("Tab") + text.Render(" next tab") + sep +
		key.Render("Shift+Tab") + text.Render(" previous tab") + sep +
		key.Render("t") + text.Render(" theme creator") + sep +
		key.Render("Ctrl+b n") + text.Render(" next track")
}
