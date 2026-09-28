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

	x, _, _ := m.fastCreateBox(width)
	lines = append(lines, "")
	for _, l := range m.framedButton(fastTracksCreate, s.fastHover == fastCreate) {
		lines = append(lines, strings.Repeat(" ", x)+l)
	}
	return lines
}

// framedButton draws label in a frame, 3 lines and the label plus 4
// cells wide; lit frames it in the focus colour.
func (m Model) framedButton(label string, lit bool) []string {
	w := lipgloss.Width(label) + 4
	border := m.fg(theme.BorderDefault)
	if lit {
		border = m.fg(theme.BorderFocus)
	}
	return []string{
		border.Render("╭" + strings.Repeat("─", w-2) + "╮"),
		border.Render("│") + m.fg(theme.TextDefault).Render(" "+label+" ") + border.Render("│"),
		border.Render("╰" + strings.Repeat("─", w-2) + "╯"),
	}
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
