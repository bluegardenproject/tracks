package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// The section list takes a quarter of the width, the section the rest;
// narrower windows show whichever has focus.
const (
	settingsSideMin = 20
	themeFieldWidth = 32
	keyWidth        = 14
	aboutLabelWidth = 16
)

// settingsPanes is where the two frames are.
type settingsPanes struct {
	side, section bool
	sideX, sideW  int
	secX, secW    int
}

func (m Model) settingsPanes() settingsPanes {
	total := max(0, m.width-2*stationLeft)
	side := total / 4
	sec := total - side - frameGap
	switch {
	case side >= settingsSideMin && sec >= detailsMin:
		return settingsPanes{true, true, stationLeft, side, stationLeft + side + frameGap, sec}
	case m.settings.editing:
		return settingsPanes{section: true, secX: stationLeft, secW: total}
	}
	return settingsPanes{side: true, sideX: stationLeft, sideW: total}
}

// sectionWidth and sectionHeight are the section frame's inside.
func (m Model) sectionWidth() int {
	p := m.settingsPanes()
	w := p.secW
	if !p.section {
		w = max(0, m.width-2*stationLeft)
	}
	return max(0, w-4)
}

func (m Model) sectionHeight() int { return max(0, m.contentHeight()-2) }

// bodyTop is the first line inside the frames.
func (m Model) bodyTop() int { return m.contentTop() + 1 }

func (m Model) settingsView(width, height int) []string {
	p := m.settingsPanes()
	s := m.settings
	var left, right []string
	if p.side {
		left = m.frame("Settings", theme.BorderDefault, m.sectionList(p.sideW-4), p.sideW, height)
	}
	if p.section {
		right = m.frame(sectionTitles[s.section], theme.BorderDefault, m.sectionBody(p.secW-4, height-2), p.secW, height)
	}
	margin := strings.Repeat(" ", stationLeft)
	lines := make([]string, height)
	for i := range lines {
		line := margin
		if left != nil {
			line += left[i]
		}
		if left != nil && right != nil {
			line += strings.Repeat(" ", frameGap)
		}
		if right != nil {
			line += right[i]
		}
		lines[i] = pad(line, width)
	}
	return lines
}

// sectionList stacks a button per section, a blank line apart.
func (m Model) sectionList(width int) []string {
	s := m.settings
	var lines []string
	for i, title := range sectionTitles {
		if i > 0 {
			lines = append(lines, "")
		}
		label := cut(title, width-2) + strings.Repeat(" ", max(0, width-2-lipgloss.Width(title)))
		lines = append(lines, m.listItem(label, i == s.hover, i == s.section))
	}
	return lines
}

// listItem draws an entry of a list of buttons, such as the Settings
// sidebar's sections.
func (m Model) listItem(label string, hover, active bool) string {
	bg, fg := theme.ListItemBgDefault, theme.ListItemTextDefault
	switch {
	case active:
		bg, fg = theme.ListItemBgActive, theme.ListItemTextActive
	case hover:
		bg, fg = theme.ListItemBgHover, theme.ListItemTextHover
	}
	return lipgloss.NewStyle().Background(m.palette.Color(bg)).Foreground(m.palette.Color(fg)).
		Bold(active).Render(" " + label + " ")
}

func (m Model) sectionBody(width, height int) []string {
	switch m.settings.section {
	case sectionGeneral:
		return m.general(width)
	case sectionCreator:
		return strings.Split(m.settings.creator.View(), "\n")
	case sectionKeys:
		lines := m.keyLines()
		return lines[min(m.settings.keysOffset, len(lines)):min(len(lines), m.settings.keysOffset+height)]
	case sectionFastTracks:
		return m.fastTracks(width)
	}
	return m.about(width)
}

// generalIntro is General's text above the theme field.
func (m Model) generalIntro(width int) []string {
	about := "Colours for every Tracks window. Pick a theme file from " + m.themesDir + ", or make one in the Theme Creator."
	if m.themesDir == "" {
		about = "Colours for every Tracks window."
	}
	lines := []string{m.fg(theme.TextDefault).Render("Theme")}
	lines = append(lines, strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(about), "\n")...)
	return append(lines, "")
}

func (m Model) general(width int) []string {
	lines := append(m.generalIntro(width), m.themeField(width))
	if err := m.settings.themesErr; err != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the themes folder: "+err.Error(), width)))
	}
	return lines
}

// themeFieldWidth is the width inside the theme field's brackets.
func (m Model) themeFieldWidth() int { return max(4, min(themeFieldWidth, m.sectionWidth()-2)) }

// themeField shows the theme in use, as its file, with its name beside.
func (m Model) themeField(width int) string {
	s := m.settings
	t := m.palette.Theme()
	e := theme.Entry{Theme: t}
	if i := m.themeIndex(t); i >= 0 {
		e = s.themes[i]
	}
	bracket := theme.BorderDefault
	if s.fieldHover || (s.editing && s.section == sectionGeneral) {
		bracket = theme.BorderFocus
	}
	b := m.fg(bracket)
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.InputBg))
	fw := m.themeFieldWidth()
	field := b.Render("[") + bg.Foreground(m.palette.Color(theme.TextDefault)).Render(pad(" "+cut(fileName(e), fw-4), fw-2)) +
		bg.Foreground(m.palette.Color(theme.TextMuted)).Render("▾ ") + b.Render("]")
	name := t.DisplayName
	if t.BuiltIn {
		name += ", built-in"
	}
	return field + m.fg(theme.TextFaint).Render(cut("  "+name, max(0, width-fw-2)))
}

// keyLines are the Keys section's lines, before scrolling.
func (m Model) keyLines() []string {
	var lines []string
	for i, g := range keyGroups() {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.fg(theme.TextDefault).Bold(true).Render(g.title))
		for _, k := range g.keys {
			lines = append(lines, m.fg(theme.TextAccent).Render(pad(k.key, keyWidth))+m.fg(theme.TextMuted).Render(k.help))
		}
	}
	return lines
}

func (m Model) about(width int) []string {
	lines := []string{
		m.fg(theme.TextFaint).Render(pad("Version", aboutLabelWidth)) + m.fg(theme.TextDefault).Render(m.version),
	}
	for _, f := range m.aboutFacts {
		lines = append(lines, m.fg(theme.TextFaint).Render(pad(f[0], aboutLabelWidth))+
			m.fg(theme.TextMuted).Render(shorten(f[1], width-aboutLabelWidth)))
	}
	return lines
}

// onThemeField reports whether cell x, y is on General's theme field.
func (m Model) onThemeField(x, y int) bool {
	p := m.settingsPanes()
	left := p.secX + 2
	return p.section && m.settings.section == sectionGeneral &&
		y == m.bodyTop()+len(m.generalIntro(m.sectionWidth())) && x >= left && x < left+m.themeFieldWidth()+2
}

// sectionAt returns the section button drawn at cell x, y.
func (m Model) sectionAt(x, y int) (int, bool) {
	p := m.settingsPanes()
	row := y - m.bodyTop()
	if !p.side || x < p.sideX+2 || x >= p.sideX+p.sideW-2 || row < 0 || row%2 != 0 || row/2 >= len(sectionTitles) {
		return 0, false
	}
	return row / 2, true
}

// inSection reports whether x, y is inside the section frame, and
// where relative to its inside.
func (m Model) inSection(x, y int) (bx, by int, ok bool) {
	p := m.settingsPanes()
	top := m.bodyTop()
	if !p.section || x < p.secX || x >= p.secX+p.secW || y < top || y >= top+m.sectionHeight() {
		return 0, 0, false
	}
	return x - p.secX - 2, y - top, true
}

func (m Model) settingsClick(x, y int) (Model, tea.Cmd) {
	s := &m.settings
	s.notice = notice{}
	if i, ok := m.sectionAt(x, y); ok {
		if i == s.section && !s.editing {
			return m, nil
		}
		return m.settingsRequest(settingsLeave{section: i})
	}
	bx, by, ok := m.inSection(x, y)
	if !ok {
		return m, nil
	}
	if m.fastButtonAt(x, y) != fastNone {
		s.notice = notice{text: fastTracksLater}
		return m, nil
	}
	switch s.section {
	case sectionGeneral:
		s.editing = true
		if m.onThemeField(x, y) {
			return m.openPicker(pickUse)
		}
	case sectionCreator:
		var focus tea.Cmd
		if !s.editing {
			s.editing = true
			focus = s.creator.FocusHere()
		}
		var cmd tea.Cmd
		s.creator, cmd = s.creator.Update(tea.MouseClickMsg{X: bx, Y: by, Button: tea.MouseLeft})
		return m, tea.Batch(focus, cmd)
	case sectionKeys:
		s.editing = true
	}
	return m, nil
}

func (m Model) settingsWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	s := &m.settings
	mouse := msg.Mouse()
	step := 1
	if mouse.Button == tea.MouseWheelUp {
		step = -1
	}
	switch s.section {
	case sectionCreator:
		if bx, by, ok := m.inSection(mouse.X, mouse.Y); ok {
			var cmd tea.Cmd
			s.creator, cmd = s.creator.Update(tea.MouseWheelMsg{X: bx, Y: by, Button: mouse.Button})
			return m, cmd
		}
	case sectionKeys:
		s.keysOffset = max(0, min(s.keysOffset+3*step, len(m.keyLines())-m.sectionHeight()))
	}
	return m, nil
}

func (m Model) settingsHover(x, y int) Model {
	s := &m.settings
	s.hover = -1
	if i, ok := m.sectionAt(x, y); ok {
		s.hover = i
	}
	s.fieldHover = m.onThemeField(x, y)
	s.fastHover = m.fastButtonAt(x, y)
	if s.section == sectionCreator {
		bx, by, ok := m.inSection(x, y)
		if !ok {
			bx, by = -1, -1
		}
		s.creator, _ = s.creator.Update(tea.MouseMotionMsg{X: bx, Y: by})
	}
	return m
}

// pickerBox is where the picker is drawn: centred on the window.
func (m Model) pickerBox() (x, y, w, h int) {
	w, h = m.settings.picker.Size(m.width, m.height)
	return (m.width - w) / 2, (m.height - h) / 2, w, h
}

// pickerMouse handles the mouse while the picker is open: it takes
// every event, and a click outside closes it.
func (m Model) pickerMouse(msg tea.Msg) (Model, tea.Cmd) {
	p := m.settings.picker
	x, y, w, h := m.pickerBox()
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		if mouse.X < x || mouse.X >= x+w || mouse.Y < y || mouse.Y >= y+h {
			return m.picked(widget.PickerClosed)
		}
		return m.picked(p.Click(mouse.X-x, mouse.Y-y, w))
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		p.Hover(mouse.X-x, mouse.Y-y, w)
	case tea.MouseWheelMsg:
		step := 1
		if msg.Mouse().Button == tea.MouseWheelUp {
			step = -1
		}
		p.Wheel(step)
	}
	return m, nil
}

// withPicker draws the open picker over content.
func (m Model) withPicker(content string) string {
	if m.settings.picker == nil {
		return content
	}
	x, y, w, h := m.pickerBox()
	box := m.settings.picker.View(m.palette, w, h)
	return lipgloss.NewCompositor(lipgloss.NewLayer(content), lipgloss.NewLayer(box).X(x).Y(y).Z(1)).Render()
}
