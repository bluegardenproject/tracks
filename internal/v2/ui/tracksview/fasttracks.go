package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// Fast Tracks aren't built yet: the section shows its empty state, and
// its buttons only say so.
const (
	fastTracksAbout  = "Fast Tracks create new Tracks from a user defined template for speedy workflows directly from the Stations window."
	fastTracksCreate = "Create your first Fast Track now"
	fastTracksLater  = "Fast Tracks aren't built yet."
)

// Fast Tracks buttons, for hover.
const (
	fastNone = iota
	fastNew
	fastCreate
)

func (m Model) fastTracksAbout(width int) []string {
	return strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(fastTracksAbout), "\n")
}

// fastCreateBox is the framed button's size and where it starts, in
// the section's inside.
func (m Model) fastCreateBox(width int) (x, y, w int) {
	w = lipgloss.Width(fastTracksCreate) + 4
	return max(0, (width-w)/2), 2 + len(m.fastTracksAbout(width)) + 1, w
}

func (m Model) fastTracks(width int) []string {
	s := m.settings
	button := newButton
	button.Hover = s.fastHover == fastNew
	lines := append([]string{button.View(m.palette), ""}, m.fastTracksAbout(width)...)

	x, _, w := m.fastCreateBox(width)
	border := m.fg(theme.BorderDefault)
	if s.fastHover == fastCreate {
		border = m.fg(theme.BorderFocus)
	}
	indent := strings.Repeat(" ", x)
	return append(lines, "",
		indent+border.Render("╭"+strings.Repeat("─", w-2)+"╮"),
		indent+border.Render("│")+m.fg(theme.TextDefault).Render(" "+fastTracksCreate+" ")+border.Render("│"),
		indent+border.Render("╰"+strings.Repeat("─", w-2)+"╯"),
	)
}

// fastButtonAt returns the Fast Tracks button at cell x, y.
func (m Model) fastButtonAt(x, y int) int {
	p := m.settingsPanes()
	if !p.section || m.settings.section != sectionFastTracks {
		return fastNone
	}
	bx, by := x-p.secX-2, y-m.bodyTop()
	if by == 0 && bx >= 0 && bx < newButton.Width() {
		return fastNew
	}
	cx, cy, w := m.fastCreateBox(m.sectionWidth())
	if by >= cy && by < cy+3 && bx >= cx && bx < cx+w {
		return fastCreate
	}
	return fastNone
}
