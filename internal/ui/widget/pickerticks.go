package widget

import "slices"

// A ticking picker's marks, markWidth wide.
const (
	tickedMark   = "[x] "
	untickedMark = "[ ] "
)

// footer is the lines under a ticking picker's rows: a blank one and
// the OK button.
func (p Picker) footer() int {
	if p.Ticked == nil {
		return 0
	}
	return 2
}

func (p Picker) okButton() Button {
	b := NewButton("OK", ButtonAccent)
	b.Hover = p.hoverOK
	return b
}

// okAt reports whether x, y, relative to the box, is on the OK button.
func (p Picker) okAt(x, y int) bool {
	return p.Ticked != nil && y == p.rows+2 && x >= 2 && x < 2+p.okButton().Width()
}

// tick flips item i, unless it has a problem.
func (p *Picker) tick(i int) {
	if i < 0 || i >= len(p.Ticked) || p.Items[i].Problem != "" {
		return
	}
	p.Ticked = slices.Clone(p.Ticked)
	p.Ticked[i] = !p.Ticked[i]
}
