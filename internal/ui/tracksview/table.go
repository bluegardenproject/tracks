package tracksview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// columnGap is the gap between a table's columns.
const columnGap = 2

// table is rows under a header, drawn over the full width: the first
// column gets half the spare room and is cut when there is none.
type table struct {
	header []string
	rows   [][]string
	right  int // the column drawn right-aligned, -1 for none
	// selected and hover are row indexes, -1 for none; offset is the
	// first row shown.
	selected, hover, offset int
	// cell, when set, draws a cell itself on the row's fill, as wide as
	// its value; "" draws the value. A drawn cell that doesn't fit falls
	// back to it.
	cell func(row, col int, fill lipgloss.Style) string
}

// draw renders the header and the rows that fit, width by at most
// height lines.
func (t table) draw(m Model, width, height int) []string {
	widths := columnWidths(t.header, t.rows, width)
	row := func(values []string, fill lipgloss.Style, style func(col int) lipgloss.Style, drawn func(col int) string) string {
		var b strings.Builder
		used := 0
		for i, v := range values {
			if i > 0 {
				b.WriteString(fill.Render(strings.Repeat(" ", columnGap)))
				used += columnGap
			}
			if d := drawn(i); d != "" && lipgloss.Width(d) <= widths[i] {
				b.WriteString(d + fill.Render(strings.Repeat(" ", widths[i]-lipgloss.Width(d))))
				used += widths[i]
				continue
			}
			v = cut(v, widths[i])
			if i == t.right {
				v = strings.Repeat(" ", widths[i]-lipgloss.Width(v)) + v
			}
			b.WriteString(style(i).Render(pad(v, widths[i])))
			used += widths[i]
		}
		b.WriteString(fill.Render(strings.Repeat(" ", max(0, width-used))))
		return b.String()
	}

	plain := lipgloss.NewStyle()
	none := func(int) string { return "" }
	lines := []string{row(t.header, plain, func(int) lipgloss.Style { return m.fg(theme.TableTextFaint) }, none)}
	end := min(len(t.rows), t.offset+height-1)
	for i := t.offset; i < end; i++ {
		fill := plain
		switch i {
		case t.selected:
			fill = fill.Background(m.palette.Color(theme.TableBgSelected))
		case t.hover:
			fill = fill.Background(m.palette.Color(theme.TableBgHighlight))
		}
		drawn := none
		if t.cell != nil {
			drawn = func(col int) string { return t.cell(i, col, fill) }
		}
		lines = append(lines, row(t.rows[i], fill, func(col int) lipgloss.Style {
			if col == 0 {
				return fill.Foreground(m.palette.Color(theme.TableTextDefault)).Bold(i == t.selected)
			}
			return fill.Foreground(m.palette.Color(theme.TableTextMuted))
		}, drawn))
	}
	return lines
}

// columnWidths fits the columns into width: the first one gets half the
// spare room, or is cut when there is none.
func columnWidths(header []string, rows [][]string, width int) []int {
	widths := make([]int, len(header))
	for i, c := range header {
		widths[i] = lipgloss.Width(c)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], lipgloss.Width(c))
		}
	}
	total := columnGap * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	spare := width - total
	if spare < 0 {
		widths[0] = max(4, widths[0]+spare)
		return widths
	}
	if len(widths) == 1 {
		widths[0] += spare
		return widths
	}
	widths[0] += spare / 2
	rest := spare - spare/2
	for i := 1; i < len(widths); i++ {
		widths[i] += rest / (len(widths) - 1)
	}
	widths[len(widths)-1] += rest % (len(widths) - 1)
	return widths
}

// keepVisible returns the offset that shows the selected one of n rows
// when rows fit.
func keepVisible(selected, offset, rows, n int) int {
	offset = min(offset, max(0, selected))
	if rows > 0 && selected >= offset+rows {
		offset = selected - rows + 1
	}
	return max(0, min(offset, n-rows))
}

// cut shortens s to width, ending it with an ellipsis.
func cut(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	return string(r[:max(0, width-1)]) + "…"
}
