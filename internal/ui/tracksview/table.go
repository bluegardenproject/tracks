package tracksview

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
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
	// cell, when set, draws a cell itself on the row's fill, at most
	// width cells wide; "" draws the value. A drawn cell that doesn't fit
	// falls back to it.
	cell func(row, col, width int, fill lipgloss.Style) string
	// snug are the columns that take none of the spare room, so the one
	// after sits close.
	snug []int
}

// draw renders the header and the rows that fit, width by at most
// height lines.
func (t table) draw(m Model, width, height int) []string {
	widths := columnWidths(t.header, t.rows, width, t.snug...)
	row := func(values []string, fill lipgloss.Style, style func(col int) lipgloss.Style, drawn func(col, width int) string) string {
		var b strings.Builder
		used := 0
		for i, v := range values {
			if i > 0 && widths[i] == 0 {
				continue // empty in every row: not drawn, gap and all
			}
			if i > 0 {
				b.WriteString(fill.Render(strings.Repeat(" ", columnGap)))
				used += columnGap
			}
			if d := drawn(i, widths[i]); d != "" && lipgloss.Width(d) <= widths[i] {
				b.WriteString(d + fill.Render(strings.Repeat(" ", max(0, widths[i]-lipgloss.Width(d)))))
				used += widths[i]
				continue
			}
			v = cut(v, widths[i])
			if i == t.right {
				v = strings.Repeat(" ", max(0, widths[i]-lipgloss.Width(v))) + v
			}
			b.WriteString(style(i).Render(pad(v, widths[i])))
			used += widths[i]
		}
		b.WriteString(fill.Render(strings.Repeat(" ", max(0, width-used))))
		return b.String()
	}

	plain := lipgloss.NewStyle()
	none := func(int, int) string { return "" }
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
			drawn = func(col, width int) string { return t.cell(i, col, width, fill) }
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
// spare room, the others but snug share the rest, and the first is cut
// when there is none. A column empty in every row, header too, stays 0
// wide.
func columnWidths(header []string, rows [][]string, width int, snug ...int) []int {
	widths := make([]int, len(header))
	for i, c := range header {
		widths[i] = lipgloss.Width(c)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], lipgloss.Width(c))
		}
	}
	// A column empty in every row isn't drawn, nor the gap before it.
	total := 0
	for i, w := range widths {
		if i > 0 && w > 0 {
			total += columnGap
		}
		total += w
	}
	spare := width - total
	if spare < 0 {
		widths[0] = max(4, widths[0]+spare)
		return widths
	}
	var loose []int
	for i := 1; i < len(widths); i++ {
		if widths[i] > 0 && !slices.Contains(snug, i) {
			loose = append(loose, i)
		}
	}
	if len(loose) == 0 {
		widths[0] += spare
		return widths
	}
	widths[0] += spare / 2
	rest := spare - spare/2
	for _, i := range loose {
		widths[i] += rest / len(loose)
	}
	widths[loose[len(loose)-1]] += rest % len(loose)
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

// cut shortens s to width cells, ending it with an ellipsis.
func cut(s string, width int) string {
	return widget.Cut(s, width)
}
