package tracksview

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

const (
	// titleLimit is the longest name, as the New track form takes it.
	titleLimit = 60
	// renameWidth is the Rename box's widest, renameHeight its height:
	// the input, a line under it, a blank line and the buttons.
	renameWidth  = titleLimit + 12
	renameHeight = 7
	// renameInputRow and renameButtonRow are lines of the box.
	renameInputRow  = 2
	renameButtonRow = 5
	// clearCells is the input's end, " ✕ ", which empties it.
	clearCells = 3
)

// renameTarget is a part of the Rename box the mouse can be on.
type renameTarget int

const (
	renameNowhere renameTarget = iota
	renameSave
	renameCancel
	renameClear
	renameText
)

var renameKeys = []keyHelp{{"Enter", "rename"}, {"Esc", "cancel"}}

// renameBox is the Rename box over Station, for one track.
type renameBox struct {
	id, slug, was string
	input         textinput.Model
	// offset is the first rune of the input shown, keeping the cursor
	// in view.
	offset int
	hover  renameTarget
}

// startRename opens the Rename box on t, filled with the name it shows.
func (m Model) startRename(t source.Track) (Model, tea.Cmd) {
	if _, _, w, h := m.renameBox(); w < 12 || h < renameHeight {
		return m, nil
	}
	in := widget.NewInput(titleLimit, t.Name)
	in.SetValue(t.Shown())
	in.CursorEnd()
	cmd := in.Focus()
	m.station.rename = &renameBox{id: t.ID, slug: t.Name, was: t.Shown(), input: in}
	m.station.rename.scroll(m.renameTextWidth())
	return m, cmd
}

// renameMsg handles input while the Rename box is open: it takes every
// key, paste and mouse event.
func (m Model) renameMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	r := m.station.rename
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return m.saveRename()
		case "esc":
			m.station.rename = nil
			return m, nil
		}
	case tea.MouseClickMsg:
		return m.renameClick(msg.Mouse())
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		r.hover, _ = m.renameTargetAt(mouse.X, mouse.Y)
		return m, nil
	case tea.MouseMsg:
		return m, nil
	}
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	r.scroll(m.renameTextWidth())
	return m, cmd
}

func (m Model) renameClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	r := m.station.rename
	target, col := m.renameTargetAt(mouse.X, mouse.Y)
	switch target {
	case renameSave:
		return m.saveRename()
	case renameCancel:
		m.station.rename = nil
	case renameClear:
		r.input.SetValue("")
		r.input.CursorStart()
		r.offset = 0
	case renameText:
		r.input.SetCursor(r.runeAt(col))
	case renameNowhere:
		if x, y, w, h := m.renameBox(); mouse.X < x || mouse.X >= x+w || mouse.Y < y || mouse.Y >= y+h {
			m.station.rename = nil
		}
	}
	return m, nil
}

// saveRename closes the box and renames the track, unless the name is
// the one it had.
func (m Model) saveRename() (tea.Model, tea.Cmd) {
	r := m.station.rename
	m.station.rename = nil
	title := strings.TrimSpace(r.input.Value())
	if title == r.was || m.setTitle == nil {
		return m, nil
	}
	shown := title
	if shown == "" {
		shown = r.slug
	}
	return m, func() tea.Msg {
		if err := m.setTitle(r.id, title); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{ok: "Renamed the track to " + shown + ".", reload: true}
	}
}

// renameBox is where the box is drawn: centred on the window.
func (m Model) renameBox() (x, y, w, h int) {
	w, h = min(m.width, renameWidth), min(m.height, renameHeight)
	return (m.width - w) / 2, (m.height - h) / 2, w, h
}

// renameTextWidth is the cells the input's text has, between "[" and
// its clear button.
func (m Model) renameTextWidth() int {
	_, _, w, _ := m.renameBox()
	return max(1, w-4-2-clearCells)
}

// renameTargetAt is what's drawn at x, y in the box and, on the text,
// the column within it.
func (m Model) renameTargetAt(x, y int) (renameTarget, int) {
	bx, by, _, _ := m.renameBox()
	col, row := x-bx-2, y-by
	tw := m.renameTextWidth()
	switch row {
	case renameInputRow:
		switch {
		case col >= 1 && col < 1+tw:
			return renameText, col - 1
		case col >= 1+tw && col < 1+tw+clearCells:
			return renameClear, 0
		}
	case renameButtonRow:
		at := 0
		for i, b := range renameButtons(renameNowhere) {
			if col >= at && col < at+b.Width() {
				return []renameTarget{renameSave, renameCancel}[i], 0
			}
			at += b.Width() + widget.ButtonGap
		}
	}
	return renameNowhere, 0
}

func renameButtons(hover renameTarget) []widget.Button {
	save, cancel := widget.NewButton("Rename", widget.ButtonAccent), widget.NewButton("Cancel", widget.ButtonDefault)
	save.Hover, cancel.Hover = hover == renameSave, hover == renameCancel
	return []widget.Button{save, cancel}
}

// scroll moves the offset so the cursor shows in width cells, and
// fills them when the text got shorter.
func (r *renameBox) scroll(width int) {
	v, pos := []rune(r.input.Value()), r.input.Position()
	r.offset = min(r.offset, pos)
	for runeCells(v[r.offset:pos])+1 > width && r.offset < pos {
		r.offset++
	}
	for r.offset > 0 && runeCells(v[r.offset-1:])+1 <= width {
		r.offset--
	}
}

// runeAt is the rune drawn at column col of the text, or the end.
func (r *renameBox) runeAt(col int) int {
	v := []rune(r.input.Value())
	at := 0
	for i := r.offset; i < len(v); i++ {
		w := lipgloss.Width(string(v[i]))
		if col < at+w {
			return i
		}
		at += w
	}
	return len(v)
}

func runeCells(rs []rune) int { return lipgloss.Width(string(rs)) }

// withRename draws the open Rename box over content.
func (m Model) withRename(content string) string {
	if m.station.rename == nil {
		return content
	}
	x, y, w, h := m.renameBox()
	if w < 12 || h < renameHeight {
		return content
	}
	return lipgloss.NewCompositor(lipgloss.NewLayer(content), lipgloss.NewLayer(m.renameView(w)).X(x).Y(y).Z(1)).Render()
}

func (m Model) renameView(w int) string {
	r := m.station.rename
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.OverlayBg))
	border := bg.Foreground(m.palette.Color(theme.OverlayBorder))
	inner := w - 4
	line := func(s string) string {
		return border.Render("│") + bg.Render(" ") + s + bg.Render(strings.Repeat(" ", max(0, inner-lipgloss.Width(s))+1)) + border.Render("│")
	}
	title := "Rename track"
	buttons := renameButtons(r.hover)
	row := buttons[0].View(m.palette) + bg.Render(strings.Repeat(" ", widget.ButtonGap)) + buttons[1].View(m.palette)
	about := bg.Foreground(m.palette.Color(theme.OverlayTextFaint)).Render(cut("Empty shows the slug, "+r.slug+".", inner))
	return strings.Join([]string{
		border.Render("╭─ ") + bg.Foreground(m.palette.Color(theme.OverlayTextDefault)).Bold(true).Render(title) +
			border.Render(" "+strings.Repeat("─", max(0, w-5-lipgloss.Width(title)))+"╮"),
		line(""),
		line(m.renameInput(m.renameTextWidth())),
		line(about),
		line(""),
		line(row),
		border.Render("╰" + strings.Repeat("─", w-2) + "╯"),
	}, "\n")
}

// renameInput is the input, width cells of text from the offset with
// the cursor's cell lit, then the clear button, in brackets.
func (m Model) renameInput(width int) string {
	r := m.station.rename
	field := lipgloss.NewStyle().Background(m.palette.Color(theme.InputBg))
	text := field.Foreground(m.palette.Color(theme.TextDefault))
	cursor := lipgloss.NewStyle().Background(m.palette.Color(theme.TextAccent)).Foreground(m.palette.Color(theme.InputBg))
	v, pos := []rune(r.input.Value()), r.input.Position()
	var b strings.Builder
	used := 0
	for i := r.offset; i < len(v); i++ {
		w := runeCells(v[i : i+1])
		if used+w > width {
			break
		}
		if i == pos {
			b.WriteString(cursor.Render(string(v[i])))
		} else {
			b.WriteString(text.Render(string(v[i])))
		}
		used += w
	}
	if pos == len(v) && used < width {
		b.WriteString(cursor.Render(" "))
		used++
	}
	if len(v) == 0 && width > 1 {
		placeholder := cut(r.slug, width-1)
		b.WriteString(field.Foreground(m.palette.Color(theme.TextFaint)).Render(placeholder))
		used += lipgloss.Width(placeholder)
	}
	b.WriteString(field.Render(strings.Repeat(" ", max(0, width-used))))
	clear := theme.TextMuted
	if r.hover == renameClear {
		clear = theme.TextDefault
	}
	bracket := lipgloss.NewStyle().Background(m.palette.Color(theme.OverlayBg)).Foreground(m.palette.Color(theme.OverlayBorderFocus))
	return bracket.Render("[") + b.String() + field.Foreground(m.palette.Color(clear)).Render(" ✕ ") + bracket.Render("]")
}
