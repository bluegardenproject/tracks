package themecreator

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const (
	nameWidth    = 28 // the longest token name and a gap; a test checks it
	previewWidth = 10
	inputWidth   = len("[#rrggbb ]")
	// The title, a blank line and the column names sit above the
	// list; a blank line, the buttons and the status below it.
	listTop     = 3
	chromeLines = listTop + 3
)

// Where a token row's parts start, in cells. Clicks use the same
// numbers.
const (
	inputX   = nameWidth
	previewX = inputX + inputWidth + 2
	// exampleX is where a group's examples, such as its buttons, start
	// on the group's line.
	exampleX = previewX + previewWidth + 2
)

func (m Model) buttonsY() int { return listTop + m.listHeight() + 1 }

func (m Model) listHeight() int { return max(1, m.height-chromeLines) }

// scrollToFocus keeps the focused token's line in view.
func (m *Model) scrollToFocus() {
	if m.focus >= len(m.fields) || m.mode == modeName {
		return
	}
	lines, tokenLine := layout()
	line := tokenLine[m.focus]
	h := m.listHeight()
	if line-1 < m.offset {
		m.offset = max(0, line-1) // the group name above, too
	}
	if line >= m.offset+h {
		m.offset = line - h + 1
	}
	m.offset = min(m.offset, max(0, lines-h))
}

// View renders the creator, width by height cells.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	p := m.palette()
	fg := func(token theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(p.Color(token)) }

	about := "built-in, read-only: save a copy with Save as new"
	if !m.edit.BuiltIn {
		about = m.edit.ID + ".yaml"
	}
	title := fg(theme.TextDefault).Bold(true).Render(m.edit.DisplayName) + "  " + fg(theme.TextFaint).Render(about)
	if m.Dirty() {
		title += fg(theme.StateWarningText).Render("  · unsaved")
	}

	header := fg(theme.TextFaint).Render(pad("Token", nameWidth) + pad("Value", previewX-inputX) + pad("Preview", exampleX-previewX) + "Example")
	all := m.tokenList(p)
	list := all[min(m.offset, len(all)):min(len(all), m.offset+m.listHeight())]
	for len(list) < m.listHeight() {
		list = append(list, "")
	}

	status := fg(theme.TextMuted)
	if m.statusErr {
		status = fg(theme.StateDangerText)
	}
	lines := append([]string{cut(title, m.width), "", header}, list...)
	lines = append(lines, "", m.bottom(p), status.Render(cut(m.status, m.width)))
	for i := range lines {
		lines[i] = pad(cut(lines[i], m.width), m.width)
	}
	return strings.Join(lines[:min(len(lines), m.height)], "\n")
}

func (m Model) tokenList(p style.Palette) []string {
	preview := style.New(m.draft())
	lines, tokenLine := layout()
	out := make([]string, lines)
	// examples are a group's example lines, from its heading on.
	var examples []string
	group := ""
	for i, token := range theme.All {
		line := tokenLine[i]
		if g := groupOf(token); g != group {
			group = g
			out[line-1] = lipgloss.NewStyle().Foreground(p.Color(theme.TextDefault)).Bold(true).Render(g)
			switch g {
			case buttonGroup:
				examples = []string{buttonSamples(preview, m.sampleHover)}
			case stateGroup:
				examples = []string{stateSamples(preview, false), stateSamples(preview, true)}
			}
			if len(examples) > 0 {
				out[line-1] = pad(out[line-1], exampleX) + examples[0]
				examples = examples[1:]
			}
		}
		name := lipgloss.NewStyle().Foreground(p.Color(theme.TextMuted))
		if m.focused && m.focus == i && m.mode == modeEdit {
			name = lipgloss.NewStyle().Foreground(p.Color(theme.TextAccent))
		}
		out[line] = name.Render(pad(string(token), nameWidth)) + m.input(p, i) + "  " + sample(preview, token)
		if len(examples) > 0 {
			out[line] = pad(out[line], exampleX) + examples[0]
			examples = examples[1:]
		}
	}
	return out
}

// The groups whose heading shows samples: every kind of button, and a
// badge per state.
const (
	buttonGroup = "button"
	stateGroup  = "state"
)

// states are each state's tokens: soft, then as a badge.
var states = []struct {
	name                           string
	bg, text, bgAccent, textAccent theme.Token
}{
	{"success", theme.StateSuccessBg, theme.StateSuccessText, theme.StateSuccessBgAccent, theme.StateSuccessTextAccent},
	{"warning", theme.StateWarningBg, theme.StateWarningText, theme.StateWarningBgAccent, theme.StateWarningTextAccent},
	{"danger", theme.StateDangerBg, theme.StateDangerText, theme.StateDangerBgAccent, theme.StateDangerTextAccent},
	{"info", theme.StateInfoBg, theme.StateInfoText, theme.StateInfoBgAccent, theme.StateInfoTextAccent},
}

// stateSamples draws every state in the draft's colours: text on its
// soft background, or with accent, as a badge.
func stateSamples(preview style.Palette, accent bool) string {
	parts := make([]string, len(states))
	for i, s := range states {
		bg, text := s.bg, s.text
		if accent {
			bg, text = s.bgAccent, s.textAccent
		}
		parts[i] = lipgloss.NewStyle().Background(preview.Color(bg)).Foreground(preview.Color(text)).Bold(accent).Render(" " + s.name + " ")
	}
	return strings.Join(parts, strings.Repeat(" ", widget.ButtonGap))
}

// buttonSamples draws one button of each kind in the draft's colours;
// hover is the one under the mouse, -1 for none.
func buttonSamples(preview style.Palette, hover int) string {
	row, _ := widget.ButtonRow(preview, sampleButtons(hover)...)
	return row
}

func sampleButtons(hover int) []widget.Button {
	b := []widget.Button{
		widget.NewButton("default", widget.ButtonDefault),
		widget.NewButton("accent", widget.ButtonAccent),
		widget.NewButton("danger", widget.ButtonDanger),
	}
	if hover >= 0 && hover < len(b) {
		b[hover].Hover = true
	}
	return b
}

// sample shows token as it will look.
func sample(palette style.Palette, token theme.Token) string {
	c := palette.Color(token)
	switch kindOf(token) {
	case kindText:
		return lipgloss.NewStyle().Background(palette.Color(backgroundOf(token))).Foreground(c).
			Render(pad(" Sample", previewWidth))
	case kindBackground:
		return lipgloss.NewStyle().Background(c).Render(strings.Repeat(" ", previewWidth))
	default:
		on := lipgloss.NewStyle().Background(palette.Color(theme.BgBase))
		return lipgloss.NewStyle().Background(c).Render("   ") + on.Foreground(c).Render(pad(" Aa", previewWidth-3))
	}
}

func (m Model) input(p style.Palette, i int) string {
	f := m.fields[i]
	bracket := theme.BorderDefault
	switch {
	case !theme.ValidHex(value(f)):
		bracket = theme.StateDangerText
	case m.focused && i == m.focus && m.mode == modeEdit:
		bracket = theme.BorderFocus
	}
	return widget.Input(p, f.input, len("#rrggbb")+1, bracket)
}

// bottom is the row below the list: the mode's buttons, with the name
// input or the question in front of them.
func (m Model) bottom(p style.Palette) string {
	switch m.mode {
	case modeName:
		bracket := theme.BorderDefault
		if m.focused && m.focus == 0 {
			bracket = theme.BorderFocus
		}
		return m.prefix(p) + widget.Input(p, m.name, nameInputWidth, bracket) + strings.Repeat(" ", widget.ButtonGap) + m.buttonRow(p)
	case modeLeave:
		return m.prefix(p) + m.buttonRow(p)
	}
	return m.buttonRow(p)
}

const (
	nameInputWidth = 24
	namePrompt     = "Display name "
)

// prefix is what stands before the buttons in the name and leave modes.
func (m Model) prefix(p style.Palette) string {
	switch m.mode {
	case modeName:
		return lipgloss.NewStyle().Foreground(p.Color(theme.TextDefault)).Render(namePrompt)
	case modeLeave:
		return lipgloss.NewStyle().Foreground(p.Color(theme.StateWarningText)).Render("Unsaved changes. ")
	}
	return ""
}

// buttonsX is where the first button starts.
func (m Model) buttonsX() int {
	x := lipgloss.Width(m.prefix(m.palette()))
	if m.mode == modeName {
		x += nameInputWidth + 2 + widget.ButtonGap
	}
	return x
}

func (m Model) buttonRow(p style.Palette) string {
	focused, onButton := m.focusedButton()
	var row []widget.Button
	for i, b := range m.buttons() {
		button := widget.NewButton(buttonLabels[b], widget.ButtonDefault)
		button.Hover = (m.focused && onButton && b == focused) || i == m.buttonHover
		row = append(row, button)
	}
	line, _ := widget.ButtonRow(p, row...)
	return line
}

// buttonWidth is a button's width: its label and padding.
func buttonWidth(b button) int {
	return widget.NewButton(buttonLabels[b], widget.ButtonDefault).Width()
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// cut shortens a styled line to width cells.
func cut(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
