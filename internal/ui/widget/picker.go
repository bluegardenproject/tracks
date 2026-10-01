package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
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
	PickerRetry                      // Enter on an error Message: load the items again
)

// Picker is a framed list to choose one item from, drawn over the
// screen. Marked is the current choice, -1 for none.
//
// With Ticked set, one entry per item, it's a list to tick any number
// of items in instead: Space or a click ticks one, and OK or Enter
// reports PickerChosen.
type Picker struct {
	Title  string
	Items  []PickerItem
	Marked int
	Cursor int // an index into Items
	// Filter lets typing narrow the list to items whose label or
	// detail contains the text; the arrow keys still move.
	Filter bool
	// Message replaces the rows while it's set, such as "Loading…";
	// Problem makes it an error that Enter retries.
	Message string
	Problem bool
	Ticked  []bool
	query   string
	hover   int
	hoverOK bool
	offset  int // into the shown items
	rows    int // rows shown, from the last Size
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

// shown are the indexes of the items that match the filter.
func (p Picker) shown() []int {
	q := strings.ToLower(p.query)
	var out []int
	for i, it := range p.Items {
		if q == "" || strings.Contains(strings.ToLower(it.Label+" "+it.Detail), q) {
			out = append(out, i)
		}
	}
	return out
}

// Size is the box's size on a screen of width by height cells. It
// fixes how many rows show, so call it before drawing or clicking.
func (p *Picker) Size(width, height int) (w, h int) {
	w = pickerMinWidth
	label := p.labelWidth()
	for _, it := range p.Items {
		w = max(w, 4+markWidth+label+pickerGap+lipgloss.Width(p.detail(it)))
	}
	w = max(0, min(max(w, lipgloss.Width(p.Message)+4), width-4))
	p.rows = max(1, min(len(p.Items), height-2-pickerChrome-p.footer()))
	p.scroll()
	return w, min(height, p.rows+pickerChrome+p.footer())
}

// Key handles a key press.
func (p *Picker) Key(key string) PickerResult {
	if p.Message != "" {
		switch {
		case key == "esc":
			return PickerClosed
		case key == "enter" && p.Problem:
			return PickerRetry
		}
		return PickerOpen
	}
	if p.Ticked != nil {
		switch key {
		case "space":
			p.tick(p.Cursor)
			return PickerOpen
		case "enter":
			return PickerChosen
		}
	}
	if p.Filter {
		switch r := []rune(key); {
		case key == "backspace":
			if q := []rune(p.query); len(q) > 0 {
				p.query = string(q[:len(q)-1])
			}
			p.follow()
			return PickerOpen
		case key == "space":
			p.query += " "
			p.follow()
			return PickerOpen
		case len(r) == 1:
			p.query += key
			p.follow()
			return PickerOpen
		}
	}
	switch key {
	case "up", "k":
		p.move(-1)
	case "down", "j":
		p.move(1)
	case "enter", "space":
		return p.choose()
	case "esc":
		return PickerClosed
	}
	p.scroll()
	return PickerOpen
}

// move steps the cursor through the shown items.
func (p *Picker) move(step int) {
	shown := p.shown()
	if len(shown) == 0 {
		return
	}
	at := 0
	for i, item := range shown {
		if item == p.Cursor {
			at = i
		}
	}
	p.Cursor = shown[max(0, min(len(shown)-1, at+step))]
}

// follow keeps the cursor on a shown item after the filter changed.
func (p *Picker) follow() {
	p.offset = 0
	shown := p.shown()
	for _, i := range shown {
		if i == p.Cursor {
			p.scroll()
			return
		}
	}
	if len(shown) > 0 {
		p.Cursor = shown[0]
	}
	p.scroll()
}

// Click handles a click at x, y relative to the box, w wide.
func (p *Picker) Click(x, y, w int) PickerResult {
	if p.okAt(x, y) {
		return PickerChosen
	}
	i, ok := p.rowAt(x, y, w)
	if !ok {
		return PickerOpen
	}
	p.Cursor = i
	if p.Ticked != nil {
		p.tick(i)
		return PickerOpen
	}
	return p.choose()
}

// Hover highlights the row at x, y relative to the box, w wide.
func (p *Picker) Hover(x, y, w int) {
	p.hover = -1
	if i, ok := p.rowAt(x, y, w); ok {
		p.hover = i
	}
	p.hoverOK = p.okAt(x, y)
}

// Wheel scrolls by step rows.
func (p *Picker) Wheel(step int) {
	p.offset = max(0, min(p.offset+step, len(p.shown())-p.rows))
}

func (p *Picker) choose() PickerResult {
	if p.Message != "" || p.Cursor < 0 || p.Cursor >= len(p.Items) || p.Items[p.Cursor].Problem != "" {
		return PickerOpen
	}
	for _, i := range p.shown() {
		if i == p.Cursor {
			return PickerChosen
		}
	}
	return PickerOpen
}

func (p Picker) rowAt(x, y, w int) (int, bool) {
	shown := p.shown()
	i := p.offset + y - 1
	if p.Message != "" || x < 1 || x >= w-1 || y < 1 || y > p.rows || i >= len(shown) {
		return 0, false
	}
	return shown[i], true
}

func (p *Picker) scroll() {
	shown := p.shown()
	at := 0
	for i, item := range shown {
		if item == p.Cursor {
			at = i
		}
	}
	if at < p.offset {
		p.offset = at
	}
	if p.rows > 0 && at >= p.offset+p.rows {
		p.offset = at - p.rows + 1
	}
	p.offset = max(0, min(p.offset, len(shown)-p.rows))
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
	bg := lipgloss.NewStyle().Background(pal.Color(theme.OverlayBg))
	border := bg.Foreground(pal.Color(theme.OverlayBorder))
	title := p.Title
	if p.query != "" {
		title += " · " + p.query
	}
	title = cutTo(title, w-6)
	lines := []string{border.Render("╭─ " + title + " " + strings.Repeat("─", max(0, w-5-lipgloss.Width(title))) + "╮")}
	body := func(s string, fill lipgloss.Style) string {
		return border.Render("│") + fill.Render(" ") + s + fill.Render(strings.Repeat(" ", max(0, w-3-lipgloss.Width(s)))) + border.Render("│")
	}
	inner := w - 4
	shown := p.shown()
	for r := range p.rows {
		switch {
		case r == 0 && p.Message != "":
			color := theme.OverlayTextMuted
			if p.Problem {
				color = theme.StateDangerText
			}
			lines = append(lines, body(bg.Foreground(pal.Color(color)).Render(cutTo(p.Message, inner)), bg))
		case r == 0 && len(shown) == 0:
			lines = append(lines, body(bg.Foreground(pal.Color(theme.OverlayTextFaint)).Render(cutTo("Nothing matches.", inner)), bg))
		case p.Message != "" || p.offset+r >= len(shown):
			lines = append(lines, body("", bg))
		default:
			row, fill := p.row(pal, shown[p.offset+r], inner, bg)
			lines = append(lines, body(row, fill))
		}
	}
	if p.Ticked != nil {
		lines = append(lines, body("", bg), body(p.okButton().View(pal), bg))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return strings.Join(lines, "\n")
}

// row draws item i, inner cells wide, on bg, and the fill it used.
func (p Picker) row(pal style.Palette, i, inner int, bg lipgloss.Style) (string, lipgloss.Style) {
	it := p.Items[i]
	fill := bg
	switch i {
	case p.Cursor:
		fill = fill.Background(pal.Color(theme.OverlayBgSelected))
	case p.hover:
		fill = fill.Background(pal.Color(theme.OverlayBgHover))
	}
	mark := strings.Repeat(" ", markWidth)
	switch {
	case i < len(p.Ticked) && p.Ticked[i]:
		mark = tickedMark
	case p.Ticked != nil:
		mark = untickedMark
	case i == p.Marked:
		mark = pickerMark
	}
	name := fill.Foreground(pal.Color(theme.OverlayTextDefault)).Bold(i == p.Cursor)
	detail := fill.Foreground(pal.Color(theme.OverlayTextMuted))
	if it.Problem != "" {
		name = fill.Foreground(pal.Color(theme.OverlayTextFaint))
		detail = fill.Foreground(pal.Color(theme.StateDangerText))
	}
	label := p.labelWidth()
	room := max(0, inner-markWidth-label-pickerGap)
	row := fill.Foreground(pal.Color(theme.OverlayTextMuted)).Render(mark) +
		name.Render(padTo(cutTo(it.Label, label), label+pickerGap)) + detail.Render(cutTo(p.detail(it), room))
	return row + fill.Render(strings.Repeat(" ", max(0, inner-lipgloss.Width(row)))), fill
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
