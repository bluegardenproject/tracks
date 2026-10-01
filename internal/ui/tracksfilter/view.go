package tracksfilter

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// Width is the popup's width in cells.
const Width = 64

// bodyRows is how many lines the frame holds.
const bodyRows = 14

// Height is the popup's height: the body in its frame, and the hint row.
func Height() int { return bodyRows + 3 }

// dateWidth is a date field's width between its brackets.
const dateWidth = 12

// placed is where a control is drawn in the body: row, and col to
// col+width.
type placed struct{ control, row, col, width int }

// body draws the popup's lines inside the frame, and where each control
// is.
func (m Model) body() ([]string, []placed) {
	var lines []string
	var at []placed
	fg := func(t theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(m.palette.Color(t)) }
	heading := func(s string) { lines = append(lines, fg(theme.TextDefault).Bold(true).Render(s)) }
	// row lays items out two cells apart on a new line.
	row := func(items ...[2]any) {
		var b strings.Builder
		col := 0
		for i, it := range items {
			if i > 0 {
				b.WriteString("  ")
				col += 2
			}
			idx, text := it[0].(int), it[1].(string)
			at = append(at, placed{idx, len(lines), col, lipgloss.Width(text)})
			b.WriteString(text)
			col += lipgloss.Width(text)
		}
		lines = append(lines, b.String())
	}
	index := func(c control) int { return slices.Index(controls, c) }
	box := func(i int, on bool, label string) [2]any { return [2]any{i, m.mark(i, on, "[x]", "[ ]", label)} }
	radio := func(i int, on bool, label string) [2]any { return [2]any{i, m.mark(i, on, "(•)", "( )", label)} }

	heading("Track status")
	var items [][2]any
	for i, s := range statusOrder {
		items = append(items, box(index(control{kindStatus, i}), m.statuses[s.ID], s.Label))
	}
	row(items...)
	lines = append(lines, "")

	heading("PR status")
	items = nil
	for i, id := range track.FilterPRStatuses {
		items = append(items, box(index(control{kindPR, i}), m.prs[id], track.PRFilterLabel(id)))
	}
	row(items...)
	lines = append(lines, "")
	row(box(index(control{kind: kindArchived}), m.archived, "Archived only"))
	lines = append(lines, "")

	heading("Started")
	items = nil
	for i, c := range startedChoices[:4] {
		items = append(items, radio(index(control{kindStarted, i}), m.started == c.value, c.label))
	}
	row(items...)
	between := index(control{kindStarted, 4})
	row(radio(between, m.started == track.Between, "Between"),
		[2]any{index(control{kind: kindFrom}), fg(theme.TextFaint).Render("From ") + m.date(kindFrom)},
		[2]any{index(control{kind: kindTo}), fg(theme.TextFaint).Render("To ") + m.date(kindTo)})
	lines = append(lines, "")

	note := fg(theme.TextFaint).Render("None ticked in a group means all of it.")
	if m.err != "" {
		note = fg(theme.StateDangerText).Render(m.err)
	}
	lines = append(lines, note)
	var buttons []widget.Button
	for _, b := range []struct {
		kind  int
		label string
		style widget.ButtonKind
	}{{kindApply, "Apply", widget.ButtonAccent}, {kindClear, "Clear", widget.ButtonDefault}, {kindCancel, "Cancel", widget.ButtonDefault}} {
		i := index(control{kind: b.kind})
		button := widget.NewButton(b.label, b.style)
		button.Hover = i == m.hover || i == m.focus
		buttons = append(buttons, button)
	}
	buttonRow, starts := widget.ButtonRow(m.palette, buttons...)
	for j, s := range starts {
		at = append(at, placed{index(control{kind: kindApply + j}), len(lines), s, buttons[j].Width()})
	}
	lines = append(lines, buttonRow)
	return lines, at
}

// mark draws a checkbox or radio button: its mark, lit when focused,
// then its label.
func (m Model) mark(i int, on bool, yes, no, label string) string {
	mark, color := no, theme.BorderDefault
	if on {
		mark = yes
	}
	if i == m.focus {
		color = theme.BorderFocus
	}
	text := lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextDefault)).Underline(i == m.hover)
	return lipgloss.NewStyle().Foreground(m.palette.Color(color)).Render(mark) + " " + text.Render(label)
}

// date draws a date field, lit when focused.
func (m Model) date(kind int) string {
	bracket := theme.BorderDefault
	if controls[m.focus].kind == kind {
		bracket = theme.BorderFocus
	}
	field := m.from
	if kind == kindTo {
		field = m.to
	}
	return widget.Input(m.palette, field, dateWidth, bracket)
}

// controlAt is the control drawn at cell x, y of the popup.
func (m Model) controlAt(x, y int) (int, bool) {
	_, at := m.body()
	for _, p := range at {
		if y == p.row+1 && x >= p.col+2 && x < p.col+2+p.width {
			return p.control, true
		}
	}
	return 0, false
}

// below is the control dir rows down from the focused one, at its place
// in the row or the row's last; the focused one when there's none.
func (m Model) below(dir int) int {
	_, at := m.body()
	rows := map[int][]int{}
	cur, place := 0, 0
	for _, p := range at {
		if p.control == m.focus {
			cur, place = p.row, len(rows[p.row])
		}
		rows[p.row] = append(rows[p.row], p.control)
	}
	for r := cur + dir; r >= 0 && r < bodyRows; r += dir {
		if row := rows[r]; len(row) > 0 {
			return row[min(place, len(row)-1)]
		}
	}
	return m.focus
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	body, _ := m.body()
	lines := widget.Frame(m.palette, "Tracks filter", body, m.width, min(m.height-1, bodyRows+2), theme.OverlayBorderFocus)
	fg := func(t theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(m.palette.Color(t)) }
	lines = append(lines, " "+widget.Hints(fg(theme.TextAccent), fg(theme.TextFaint), Keys))
	return strings.Join(lines, "\n")
}
