package platform

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// What a click on a target does.
const (
	targetCollect = iota
	targetRelayStation
	targetSolve
	targetTrain
)

// target is a clickable span of the pane: row y, columns x0 to x1.
type target struct {
	y, x0, x1 int
	kind      int
	index     int // a PR's for targetSolve, a train's for targetTrain
}

// item is a clickable line of a frame's body: its row, the pane's
// column it starts at (0 for the usual 3: the frame's border and padding,
// and the line's own leading space), its width, and what it does. A
// whole item spans the frame's inside, borders apart.
type item struct {
	row, from, width, kind, index int
	whole                         bool
}

// layout is the pane's lines, the Platform's frame over Signals', and
// where its clickable things are. The Platform shows while there are
// trains; Signals fills the rest of the pane.
func (m Model) layout() ([]string, []target) {
	if m.width < 6 || m.height < 3 {
		return nil, nil
	}
	var lines []string
	var targets []target
	frame := func(title string, body []string, items []item, h int) {
		top := len(lines)
		lines = append(lines, widget.Frame(m.palette, title, body, m.width, h, theme.BorderDefault)...)
		for _, it := range items {
			// The frame's border and padding take two columns, and a
			// clickable line starts one in.
			x0, x1 := 3, 0
			if it.from > 0 {
				x0 = it.from
			}
			x1 = min(x0+it.width, m.width-2)
			if it.whole {
				x0, x1 = 1, m.width-1
			}
			targets = append(targets, target{y: top + 1 + it.row, x0: x0, x1: x1, kind: it.kind, index: it.index})
		}
	}
	if len(m.trains) > 0 || m.err != nil {
		body, items := m.platformBody()
		frame("Platform", body, items, len(body)+2)
	}
	body, items := m.signalsBody()
	frame("Signals", body, items, max(len(body)+2, m.height-len(lines)))
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return lines, targets
}

// platformBody is the Platform frame's inside: each train, its badges
// and its Collect messages.
func (m Model) platformBody() ([]string, []item) {
	inner := m.width - 4
	var body []string
	var items []item
	for i, t := range m.trains {
		// Its rows, name and badges, open its overlay, from border to
		// border.
		name := cutTo(t.Handle, inner)
		items = append(items, item{row: len(body), kind: targetTrain, index: i, whole: true})
		body = append(body, m.bold(name))
		badges := widget.Badge(m.palette, t.Status)
		if t.Outcome.Label != "" {
			badges += " " + widget.Badge(m.palette, t.Outcome)
		}
		items = append(items, item{row: len(body), kind: targetTrain, index: i, whole: true})
		body = append(body, " "+badges)
		if t.Held {
			b := widget.NewButton("Collect messages", widget.ButtonAccent)
			items = append(items, item{row: len(body), width: b.Width(), kind: targetCollect})
			body = append(body, " "+b.View(m.palette))
		}
		body = append(body, "")
	}
	if m.err != nil {
		body = append(body, m.muted("Couldn't read the trains: "+m.err.Error(), inner)...)
	}
	if m.notice != "" {
		body = append(body, m.muted(m.notice, inner)...)
	}
	return body, items
}

// signalsBody is the Signals frame's inside: the Relay Station and its
// count of new messages, and each PR with open comments and its button.
func (m Model) signalsBody() ([]string, []item) {
	inner := m.width - 4
	line := lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextAccent)).Render("Relay Station")
	switch n := m.signals.RelayNew; {
	case n > 99:
		line += m.mutedText(" · 99+")
	case n > 0:
		line += m.mutedText(" · " + strconv.Itoa(n) + " new")
	}
	body := []string{line, ""}
	// The whole line opens it, from the body's first column.
	items := []item{{row: 0, width: lipgloss.Width(line) + 1, kind: targetRelayStation, from: 2}}
	for _, pr := range m.signals.PRs {
		body = append(body, m.bold(cutTo(pr.PR, inner)))
		if pr.Sent {
			body = append(body, " "+m.mutedText(cutTo(pr.Action, inner-1)))
		} else {
			b := widget.NewButton(cutTo(pr.Action, inner-3), widget.ButtonDefault)
			items = append(items, item{row: len(body), width: b.Width(), kind: targetSolve, index: pr.Index})
			body = append(body, " "+b.View(m.palette))
		}
		body = append(body, "")
	}
	if len(m.trains) == 0 && m.err == nil && m.notice != "" {
		body = append(body, m.muted(m.notice, inner)...)
	}
	return body, items
}

func (m Model) bold(text string) string {
	return lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextDefault)).Bold(true).Render(text)
}

func (m Model) mutedText(text string) string {
	return lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextMuted)).Render(text)
}

// muted is text in muted colour, wrapped to width.
func (m Model) muted(text string, width int) []string {
	return strings.Split(lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextMuted)).Width(max(1, width)).Render(text), "\n")
}

// cutTo is s, cut to width cells with an ellipsis.
func cutTo(s string, width int) string {
	r := []rune(s)
	if width < 1 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
