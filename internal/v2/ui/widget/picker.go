package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
)

// PickerItem is a row of a Picker. An item with a Problem is listed
// with it, but can't be picked.
type PickerItem struct {
	Label, Detail, Problem string
}

// PickerResult is what a key or click did to a Picker.
type PickerResult int

const (
	PickerOpen   PickerResult = iota // still choosing
	PickerChosen                     // Cursor is the choice
	PickerClosed                     // closed without a choice
)

// Picker is a framed list to choose one item from, drawn over the
// screen. Marked is the current choice, -1 for none.
type Picker struct {
	Title  string
	Items  []PickerItem
	Marked int
	Cursor int
	hover  int
	offset int
	rows   int // rows shown, from the last Size
}

// The box around the rows is just its borders; the screen's hint row
// shows the picker's keys.
const (
	pickerChrome   = 2
	pickerMinWidth = 40
	pickerGap      = 2
	pickerMark     = "(•) "
	markWidth      = 4
)

// NewPicker returns a picker with the cursor on marked.
func NewPicker(title string, items []PickerItem, marked int) Picker {
	return Picker{Title: title, Items: items, Marked: marked, Cursor: max(0, marked), hover: -1}
}

// SetItems replaces the items, keeping the cursor in range.
func (p *Picker) SetItems(items []PickerItem) {
	p.Items = items
	p.Cursor = max(0, min(p.Cursor, len(items)-1))
	p.scroll()
}

// Size is the box's size on a screen of width by height cells. It
// fixes how many rows show, so call it before drawing or clicking.
func (p *Picker) Size(width, height int) (w, h int) {
	w = pickerMinWidth
	label := p.labelWidth()
	for _, it := range p.Items {
		w = max(w, 4+markWidth+label+pickerGap+lipgloss.Width(p.detail(it)))
	}
	w = max(0, min(w, width-4))
	p.rows = max(1, min(len(p.Items), height-2-pickerChrome))
	p.scroll()
	return w, min(height, p.rows+pickerChrome)
}

// Key handles a key press.
func (p *Picker) Key(key string) PickerResult {
	switch key {
	case "up", "k":
		p.Cursor = max(0, p.Cursor-1)
	case "down", "j":
		p.Cursor = min(len(p.Items)-1, p.Cursor+1)
	case "enter", "space":
		return p.choose()
	case "esc":
		return PickerClosed
	}
	p.scroll()
	return PickerOpen
}

// Click handles a click at x, y relative to the box, w wide.
func (p *Picker) Click(x, y, w int) PickerResult {
	i, ok := p.rowAt(x, y, w)
	if !ok {
		return PickerOpen
	}
	p.Cursor = i
	return p.choose()
}

// Hover highlights the row at x, y relative to the box, w wide.
func (p *Picker) Hover(x, y, w int) {
	p.hover = -1
	if i, ok := p.rowAt(x, y, w); ok {
		p.hover = i
	}
}

// Wheel scrolls by step rows.
func (p *Picker) Wheel(step int) {
	p.offset = max(0, min(p.offset+step, len(p.Items)-p.rows))
}

func (p *Picker) choose() PickerResult {
	if p.Cursor < 0 || p.Cursor >= len(p.Items) || p.Items[p.Cursor].Problem != "" {
		return PickerOpen
	}
	return PickerChosen
}

func (p Picker) rowAt(x, y, w int) (int, bool) {
	i := p.offset + y - 1
	if x < 1 || x >= w-1 || y < 1 || y > p.rows || i >= len(p.Items) {
		return 0, false
	}
	return i, true
}

func (p *Picker) scroll() {
	if p.Cursor < p.offset {
		p.offset = p.Cursor
	}
	if p.rows > 0 && p.Cursor >= p.offset+p.rows {
		p.offset = p.Cursor - p.rows + 1
	}
	p.offset = max(0, min(p.offset, len(p.Items)-p.rows))
}

func (p Picker) labelWidth() int {
	w := 0
	for _, it := range p.Items {
		w = max(w, lipgloss.Width(it.Label))
	}
	return w
}

func (p Picker) detail(it PickerItem) string {
	if it.Problem != "" {
		return it.Problem
	}
	return it.Detail
}

// View draws the box, w by h cells as Size returned.
func (p Picker) View(pal style.Palette, w, h int) string {
	if w < 6 || h < pickerChrome {
		return ""
	}
	bg := lipgloss.NewStyle().Background(pal.Color(theme.BgOverlay))
	border := bg.Foreground(pal.Color(theme.BorderDefault))
	title := cutTo(p.Title, w-6)
	lines := []string{border.Render("╭─ " + title + " " + strings.Repeat("─", max(0, w-5-lipgloss.Width(title))) + "╮")}
	body := func(s string, fill lipgloss.Style) string {
		return border.Render("│") + fill.Render(" ") + s + fill.Render(strings.Repeat(" ", max(0, w-3-lipgloss.Width(s)))) + border.Render("│")
	}
	label := p.labelWidth()
	inner := w - 4
	for r := range p.rows {
		i := p.offset + r
		if i >= len(p.Items) {
			lines = append(lines, body("", bg))
			continue
		}
		it := p.Items[i]
		fill := bg
		switch i {
		case p.Cursor:
			fill = fill.Background(pal.Color(theme.TableBgSelected))
		case p.hover:
			fill = fill.Background(pal.Color(theme.TableBgHighlight))
		}
		mark := strings.Repeat(" ", markWidth)
		if i == p.Marked {
			mark = pickerMark
		}
		name := fill.Foreground(pal.Color(theme.TableTextDefault)).Bold(i == p.Cursor)
		detail := fill.Foreground(pal.Color(theme.TableTextMuted))
		if it.Problem != "" {
			name = fill.Foreground(pal.Color(theme.TableTextFaint))
			detail = fill.Foreground(pal.Color(theme.StateDangerText))
		}
		room := max(0, inner-markWidth-label-pickerGap)
		row := fill.Foreground(pal.Color(theme.TableTextMuted)).Render(mark) +
			name.Render(padTo(cutTo(it.Label, label), label+pickerGap)) + detail.Render(cutTo(p.detail(it), room))
		lines = append(lines, body(row+fill.Render(strings.Repeat(" ", max(0, inner-lipgloss.Width(row)))), fill))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return strings.Join(lines, "\n")
}

func padTo(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

func cutTo(s string, width int) string {
	r := []rune(s)
	if width < 1 {
		return ""
	}
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
