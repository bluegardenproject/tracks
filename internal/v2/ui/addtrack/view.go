package addtrack

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// hit is where a control, or item index of it, is drawn in the body.
type hit struct {
	ctl         control
	index       int
	x, y, w, h  int
	top, bottom int // the lines of its field, heading to problem
}

var noHit = hit{ctl: -1}

// The prompt grows into the room the other fields leave.
const (
	minPrompt = 4
	maxPrompt = 14
)

// chrome is the frame's two lines and the hint row.
const chrome = 3

func (m Model) rows() int { return max(0, m.height-chrome) }

func (m Model) fg(t theme.Token) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.palette.Color(t))
}

// body draws the fields, inner cells wide.
type body struct {
	m     Model
	width int
	lines []string
	hits  []hit
	top   int // where the field being drawn starts
}

func (b *body) add(lines ...string) { b.lines = append(b.lines, lines...) }

func (b *body) hit(c control, index, x, w, h int) {
	b.hits = append(b.hits, hit{ctl: c, index: index, x: x, y: len(b.lines), w: w, h: h})
}

func (b *body) wrapped(t theme.Token, text string) {
	b.add(strings.Split(b.m.fg(t).Width(max(1, b.width)).Render(text), "\n")...)
}

// field draws c's heading and description; end closes it with its
// problem.
func (b *body) field(c control, about string) {
	b.top = len(b.lines)
	heading := b.m.fg(theme.TextDefault)
	if b.m.focus == c || c == ctlEngine && b.m.focus == ctlModel {
		heading = b.m.fg(theme.TextAccent)
	}
	b.add(heading.Bold(true).Render(title(b.m.kind, c)))
	if about != "" {
		b.wrapped(theme.TextMuted, about)
	}
}

func (b *body) end(c control) {
	if problem := b.m.errs[c]; problem != "" {
		b.wrapped(theme.StateDangerText, problem)
	}
	for i := range b.hits {
		if h := b.hits[i].ctl; h == c || c == ctlEngine && h == ctlModel {
			b.hits[i].top, b.hits[i].bottom = b.top, len(b.lines)
		}
	}
	b.add("")
}

func (m Model) body(width int) ([]string, []hit) {
	b := &body{m: m, width: width}
	b.field(ctlType, "")
	b.types()
	b.wrapped(theme.TextMuted, kinds[m.kind].about)
	b.end(ctlType)
	for _, c := range kinds[m.kind].fields {
		b.field(c, about(m.kind, c))
		switch c {
		case ctlRepos, ctlRepo:
			b.repos(c)
		case ctlTarget:
			b.input(c, m.target)
		case ctlDocument:
			b.input(c, m.document)
		case ctlName:
			b.input(c, m.name)
		case ctlTerminal:
			b.checks(c, []string{"Open a shell in the worktree, beside the agent."}, []bool{m.terminal})
		case ctlSections:
			b.checks(c, sections, m.sections)
		case ctlCandor:
			b.selectField(ctlCandor, candorLabel(m.candor), theme.TextDefault)
		case ctlPrompt:
			b.prompt()
		}
		b.end(c)
		if c == ctlName {
			b.field(ctlEngine, about(m.kind, ctlEngine))
			b.runsOnRow()
			b.end(ctlEngine)
		}
	}
	if m.failure != "" {
		b.wrapped(theme.StateDangerText, m.failure)
		b.add("")
	}
	b.buttons()
	return b.lines, b.hits
}

// types draws the Type row, a list item per kind.
func (b *body) types() {
	var row strings.Builder
	x := 0
	for i, k := range kinds {
		if i > 0 {
			row.WriteString(" ")
			x++
		}
		bg, fg := theme.ListItemBgDefault, theme.ListItemTextDefault
		switch {
		case Kind(i) == b.m.kind:
			bg, fg = theme.ListItemBgActive, theme.ListItemTextActive
		case b.m.hover.ctl == ctlType && b.m.hover.index == i:
			bg, fg = theme.ListItemBgHover, theme.ListItemTextHover
		}
		chip := " " + k.label + " "
		b.hit(ctlType, i, x, lipgloss.Width(chip), 1)
		row.WriteString(lipgloss.NewStyle().Background(b.m.palette.Color(bg)).Foreground(b.m.palette.Color(fg)).
			Bold(Kind(i) == b.m.kind).Render(chip))
		x += lipgloss.Width(chip)
	}
	b.add(row.String())
}

// repos draws the selector that opens the repos' picker: Review's shows
// its repo; the others' show the picked repos under it.
func (b *body) repos(c control) {
	m := b.m
	switch {
	case m.reposErr != nil:
		b.wrapped(theme.StateDangerText, "Couldn't read the repos: "+m.reposErr.Error())
		return
	case len(m.repos) == 0:
		b.wrapped(theme.TextFaint, noRepos)
		return
	case c == ctlRepo:
		b.selectField(c, m.repos[min(m.repo, len(m.repos)-1)], theme.TextDefault)
		return
	}
	b.selectField(c, selectRepos, theme.TextFaint)
	b.picked()
}

// picked draws the picked repos in rows, each with an ✕ that removes
// it. Their hits are the ✕s, from index 1.
func (b *body) picked() {
	m := b.m
	var row strings.Builder
	x := 0
	for i, r := range m.pickedRepos() {
		chip := " " + cut(r, max(1, b.width-4)) + " ✕ "
		w := lipgloss.Width(chip)
		if x > 0 && x+1+w > b.width {
			b.add(row.String())
			row.Reset()
			x = 0
		}
		if x > 0 {
			row.WriteString(" ")
			x++
		}
		bg, fg := theme.ListItemBgDefault, theme.ListItemTextDefault
		switch {
		case m.focus == ctlRepos && m.item == i+1:
			bg, fg = theme.ListItemBgActive, theme.ListItemTextActive
		case m.hover.ctl == ctlRepos && m.hover.index == i+1:
			bg, fg = theme.ListItemBgHover, theme.ListItemTextHover
		}
		b.hit(ctlRepos, i+1, x+w-3, 3, 1)
		row.WriteString(lipgloss.NewStyle().Background(m.palette.Color(bg)).Foreground(m.palette.Color(fg)).Render(chip))
		x += w
	}
	if x > 0 {
		b.add(row.String())
	}
}

// checks draws labels as a list to tick.
func (b *body) checks(c control, labels []string, marks []bool) {
	m := b.m
	w := 0
	for _, l := range labels {
		w = max(w, lipgloss.Width(l))
	}
	w = min(b.width, w+6)
	for i, label := range labels {
		row := lipgloss.NewStyle()
		switch {
		case m.focus == c && m.item == i:
			row = row.Background(m.palette.Color(theme.TableBgSelected))
		case m.hover.ctl == c && m.hover.index == i:
			row = row.Background(m.palette.Color(theme.TableBgHighlight))
		}
		mark, on := "[ ]", "[x]"
		markColor := theme.BorderDefault
		if marks[i] {
			mark, markColor = on, theme.TextAccent
		}
		if m.focus == c && m.item == i {
			markColor = theme.BorderFocus
		}
		text := row.Foreground(m.palette.Color(markColor)).Render(" "+mark+" ") +
			row.Foreground(m.palette.Color(theme.TextDefault)).Render(cut(label, w-6)+" ")
		b.hit(c, i, 0, w, 1)
		b.add(text + row.Render(strings.Repeat(" ", max(0, w-lipgloss.Width(text)))))
	}
}

func (b *body) bracket(c control) theme.Token {
	switch {
	case b.m.errs[c] != "":
		return theme.StateDangerText
	case b.m.focus == c:
		return theme.BorderFocus
	}
	return theme.BorderDefault
}

func (b *body) input(c control, in textinput.Model) {
	b.hit(c, 0, 0, b.width, 1)
	b.add(widget.Input(b.m.palette, in, max(1, b.width-2), b.bracket(c)))
}

// selectField draws value, in fg, in a field that opens a picker. On
// Repos it's lit only while the cursor is on it, not on a repo.
func (b *body) selectField(c control, value string, fg theme.Token) {
	b.hit(c, 0, 0, b.width, 1)
	b.add(b.selectBox(c, value, fg, b.width))
}

// selectBox is a field that opens a picker, width cells wide.
func (b *body) selectBox(c control, value string, fg theme.Token, width int) string {
	m := b.m
	bracket := b.bracket(c)
	if m.focus == c && m.item != 0 && m.errs[c] == "" {
		bracket = theme.BorderDefault
	}
	br := m.fg(bracket)
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.InputBg))
	inner := max(4, width-2)
	value = cut(" "+value, inner-2)
	return br.Render("[") + bg.Foreground(m.palette.Color(fg)).Render(value+strings.Repeat(" ", max(0, inner-2-lipgloss.Width(value)))) +
		bg.Foreground(m.palette.Color(theme.TextMuted)).Render("▾ ") + br.Render("]")
}

// prompt draws the text area in a box.
func (b *body) prompt() {
	m := b.m
	border := m.fg(b.bracket(ctlPrompt))
	inner := max(1, b.width-2)
	b.hit(ctlPrompt, 0, 0, b.width, m.prompt.Height()+2)
	b.add(border.Render("╭" + strings.Repeat("─", inner) + "╮"))
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.InputBg))
	for _, line := range strings.Split(m.styledPrompt().View(), "\n") {
		line = lipgloss.NewStyle().MaxWidth(inner).Render(line)
		b.add(border.Render("│") + line + bg.Render(strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))) + border.Render("│"))
	}
	b.add(border.Render("╰" + strings.Repeat("─", inner) + "╯"))
}

// styledPrompt is the prompt in the theme's colours.
func (m Model) styledPrompt() textarea.Model {
	t := m.prompt
	s := t.Styles()
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.InputBg))
	for _, state := range []*textarea.StyleState{&s.Focused, &s.Blurred} {
		text := bg.Foreground(m.palette.Color(theme.TextDefault))
		if state == &s.Blurred {
			text = bg.Foreground(m.palette.Color(theme.TextMuted))
		}
		state.Base, state.Text, state.CursorLine = bg, text, text
		state.Placeholder = bg.Foreground(m.palette.Color(theme.TextFaint))
		state.EndOfBuffer = bg
		state.Prompt = bg
	}
	s.Cursor.Color = m.palette.Color(theme.TextAccent)
	s.Cursor.Blink = false
	t.SetStyles(s)
	return t
}

func (b *body) buttons() {
	m := b.m
	create := widget.NewButton("Create", widget.ButtonAccent)
	cancel := widget.NewButton("Cancel", widget.ButtonDefault)
	create.Hover = m.focus == ctlCreate || m.hover.ctl == ctlCreate
	cancel.Hover = m.focus == ctlCancel || m.hover.ctl == ctlCancel
	row, starts := widget.ButtonRow(m.palette, create, cancel)
	b.top = len(b.lines)
	b.hit(ctlCreate, 0, starts[0], create.Width(), 1)
	b.hit(ctlCancel, 0, starts[1], cancel.Width(), 1)
	for i := len(b.hits) - 2; i < len(b.hits); i++ {
		b.hits[i].top, b.hits[i].bottom = b.top, b.top+1
	}
	b.add(row)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width < 20 || m.height < chrome+2 {
		return ""
	}
	lines, _ := m.body(m.width - 4)
	lines = lines[min(m.offset, len(lines)):]
	framed := widget.Frame(m.palette, "New track", lines, m.width, m.height-1, theme.OverlayBorderFocus)
	framed = append(framed, m.hints())
	screen := strings.Join(framed, "\n")
	switch {
	case m.picker != nil:
		x, y, w, h := m.pickerBox()
		return overlay(screen, m.picker.View(m.palette, w, h), x, y)
	case m.discard != nil:
		x, y, box := m.discardBox()
		return overlay(screen, strings.Join(box, "\n"), x, y)
	}
	return screen
}

func overlay(screen, box string, x, y int) string {
	return lipgloss.NewCompositor(lipgloss.NewLayer(screen), lipgloss.NewLayer(box).X(x).Y(y).Z(1)).Render()
}

// Keys are every key the form takes, for the Keys list.
var Keys = []widget.KeyHelp{
	{Key: "Tab", Help: "next field"}, {Key: "Shift+Tab", Help: "previous field"}, {Key: "←/→", Help: "type, or the picked repos"},
	{Key: "↑/↓", Help: "select"}, {Key: "Space", Help: "tick"}, {Key: "Backspace", Help: "remove a picked repo"},
	{Key: "Enter", Help: "new line in the prompt"}, {Key: "Esc", Help: "close"},
}

// hints is the bottom row: the notice, or the keys the focus takes.
func (m Model) hints() string {
	if m.creating != nil {
		return " " + m.fg(theme.StateInfoText).Render(m.notice) + "  " +
			widget.Hints(m.fg(theme.TextAccent), m.fg(theme.TextFaint), []widget.KeyHelp{{Key: "Esc", Help: "close, it goes on"}})
	}
	if m.notice != "" {
		return " " + m.fg(theme.StateInfoText).Render(m.notice)
	}
	var keys []widget.KeyHelp
	switch {
	case m.discard != nil:
		keys = []widget.KeyHelp{{Key: "←/→", Help: "choose"}, {Key: "Enter", Help: "press"}, {Key: "Esc", Help: "keep editing"}}
	case m.picker != nil && m.picker.Ticked != nil:
		keys = []widget.KeyHelp{{Key: "Type", Help: "to filter"}, {Key: "↑/↓", Help: "select"}, {Key: "Space", Help: "tick"}, {Key: "Enter", Help: "OK"}, {Key: "Esc", Help: "close"}}
	case m.picker != nil && m.picker.Filter:
		keys = []widget.KeyHelp{{Key: "Type", Help: "to filter"}, {Key: "↑/↓", Help: "select"}, {Key: "Enter", Help: "choose"}, {Key: "Esc", Help: "close"}}
	case m.picker != nil:
		keys = []widget.KeyHelp{{Key: "↑/↓", Help: "select"}, {Key: "Enter", Help: "choose"}, {Key: "Esc", Help: "close"}}
	default:
		switch m.focus {
		case ctlType:
			keys = append(keys, widget.KeyHelp{Key: "←/→", Help: "type"})
		case ctlRepos:
			keys = append(keys, m.reposHints()...)
		case ctlSections, ctlTerminal:
			keys = append(keys, widget.KeyHelp{Key: "Space", Help: "tick"})
		case ctlRepo:
			keys = append(keys, widget.KeyHelp{Key: "Enter", Help: "pick"})
		case ctlCandor:
			keys = append(keys, widget.KeyHelp{Key: "Enter", Help: "pick"})
		case ctlEngine, ctlModel:
			keys = append(keys, widget.KeyHelp{Key: "Enter", Help: "pick"}, widget.KeyHelp{Key: "←/→", Help: "agent or model"})
		case ctlPrompt:
			keys = append(keys, widget.KeyHelp{Key: "Enter", Help: "new line"})
		case ctlCreate, ctlCancel:
			keys = append(keys, widget.KeyHelp{Key: "Enter", Help: "press"})
		}
		keys = append(keys, widget.KeyHelp{Key: "Tab", Help: "next"}, widget.KeyHelp{Key: "Shift+Tab", Help: "previous"}, widget.KeyHelp{Key: "Esc", Help: "close"})
	}
	return " " + widget.Hints(m.fg(theme.TextAccent), m.fg(theme.TextFaint), keys)
}

func cut(s string, width int) string {
	r := []rune(s)
	switch {
	case width < 1:
		return ""
	case len(r) <= width:
		return s
	}
	return string(r[:width-1]) + "…"
}
