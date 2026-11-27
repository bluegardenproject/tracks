package platform

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// What a click on a target does.
const (
	targetCollect = iota
	// targetCollectSignals is Collect messages under Signals, for
	// @github's events: its notice shows there.
	targetCollectSignals
	targetRelayStation
	targetSolve
	targetTrain
	targetSide
)

// target is a clickable span of the pane: row y, columns x0 to x1.
type target struct {
	y, x0, x1 int
	kind      int
	index     int // a PR's for targetSolve, a train's for targetTrain, a side track's for targetSide
}

// item is a clickable line of a frame's body: its row, the pane's
// column it starts at (0 for the usual 3: the frame's border and padding,
// and the line's own leading space), its width, and what it does. A
// whole item spans the frame's inside, borders apart.
type item struct {
	row, from, width, kind, index int
	whole                         bool
}

// layout is the pane's lines, the Platform's frame over Signals', each
// half the pane, and where its clickable things are.
func (m Model) layout() ([]string, []target) {
	if m.width < 6 || m.height < 4 {
		return nil, nil
	}
	var lines []string
	var targets []target
	frame := func(title string, body []string, items []item, h int) {
		top := len(lines)
		lines = append(lines, widget.BorderTitledFrame(m.palette, title, body, m.width, h, theme.BorderDefault)...)
		for _, it := range items {
			if it.row >= h-2 {
				// Cut off with the rest of a body taller than its frame.
				continue
			}
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
	// The two share the pane's height half and half, Signals taking
	// an odd row.
	top := m.height / 2
	body, items := m.platformBody()
	frame("Platform", body, items, top)
	body, items = m.signalsBody()
	frame("Signals", body, items, m.height-top)
	return lines, targets
}

// platformBody is the Platform frame's inside: under a dim Trains each
// train, its badges and its Collect messages, then under a dim Side
// Tracks each side track and its badges, "empty" for none; a side track
// shows its Main Track instead, with a button back to it.
func (m Model) platformBody() ([]string, []item) {
	inner := m.width - 4
	var body []string
	var items []item
	if m.err != nil {
		body = append(body, m.muted("Couldn't read the Platform: "+m.err.Error(), inner)...)
		return body, items
	}
	body = append(body, m.mutedText("Trains"))
	for i, t := range m.trains {
		// Its rows, name and badges, open its overlay, from border to
		// border.
		name := cutTo(t.Handle, inner)
		items = append(items, item{row: len(body), kind: targetTrain, index: i, whole: true})
		body = append(body, m.item(name))
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
	if len(m.trains) == 0 {
		body = append(body, m.mutedText("empty"), "")
	}
	if m.notice != "" && m.noticeFor == targetCollect {
		body = append(body, m.muted(m.notice, inner)...)
		body = append(body, "")
	}

	if i := slices.IndexFunc(m.sides, func(s Side) bool { return s.Main }); i >= 0 {
		// A side track's way back: its main track's name, and a button.
		body = append(body, m.mutedText("Main Track"))
		items = append(items, item{row: len(body), kind: targetSide, index: i, whole: true})
		body = append(body, m.item(cutTo(m.sides[i].Name, inner)))
		b := widget.NewButton("Back to main track", widget.ButtonDefault)
		items = append(items, item{row: len(body), width: b.Width(), kind: targetSide, index: i})
		body = append(body, " "+b.View(m.palette))
		if m.notice != "" && m.noticeFor == targetSide {
			body = append(body, m.muted(m.notice, inner)...)
		}
		return body, items
	}
	body = append(body, m.mutedText("Side Tracks"))
	for i, s := range m.sides {
		// Its rows, name and badges, switch to its window.
		items = append(items, item{row: len(body), kind: targetSide, index: i, whole: true})
		body = append(body, m.item(cutTo(s.Name, inner)))
		badges := widget.Badge(m.palette, s.Status)
		pr := ""
		if s.PR.Label != "" {
			pr = widget.Badge(m.palette, s.PR)
		}
		if pr != "" && s.Status.Label != "" && lipgloss.Width(badges+" "+pr) < inner {
			badges, pr = badges+" "+pr, ""
		}
		// A long status, "action required", leaves the PR's badge a row
		// of its own rather than cut.
		for _, row := range []string{badges, pr} {
			if row != "" {
				items = append(items, item{row: len(body), kind: targetSide, index: i, whole: true})
				body = append(body, " "+row)
			}
		}
		body = append(body, "")
	}
	if len(m.sides) == 0 {
		body = append(body, m.mutedText("empty"))
	}
	if m.notice != "" && m.noticeFor == targetSide {
		body = append(body, m.muted(m.notice, inner)...)
	}
	return body, items
}

// signalsBody is the Signals frame's inside: the Relay Station and its
// count of new messages, and each PR with open comments and its button.
func (m Model) signalsBody() ([]string, []item) {
	inner := m.width - 4
	line := m.item("Relay Station")
	switch n := m.signals.RelayNew; {
	case n > 99:
		line += m.mutedText(" · ") + m.attentionText("99+")
	case n > 0:
		line += m.mutedText(" · ") + m.attentionText(strconv.Itoa(n)+" new")
	}
	body := []string{line}
	// The whole line opens it, from the body's first column.
	items := []item{{row: 0, width: lipgloss.Width(line), kind: targetRelayStation, from: 2}}
	if m.signals.Held {
		b := widget.NewButton("Collect messages", widget.ButtonAccent)
		items = append(items, item{row: len(body), width: b.Width(), kind: targetCollectSignals})
		body = append(body, " "+b.View(m.palette))
	}
	body = append(body, "")
	for _, pr := range m.signals.PRs {
		body = append(body, m.item(cutTo(pr.PR, inner)))
		if pr.Sent {
			body = append(body, " "+m.mutedText(cutTo(pr.Action, inner-1)))
		} else {
			b := widget.NewButton(cutTo(pr.Action, inner-3), widget.ButtonDefault)
			items = append(items, item{row: len(body), width: b.Width(), kind: targetSolve, index: pr.Index})
			body = append(body, " "+b.View(m.palette))
		}
		body = append(body, "")
	}
	// A notice of Signals' own clicks shows here.
	if m.notice != "" && (m.noticeFor == targetCollectSignals || m.noticeFor == targetRelayStation || m.noticeFor == targetSolve) {
		body = append(body, m.muted(m.notice, inner)...)
	}
	return body, items
}

// item is a list item's name, as the Station shows a track's.
func (m Model) item(text string) string {
	return lipgloss.NewStyle().Foreground(m.palette.Color(theme.TableTextDefault)).Render(text)
}

// attentionText is text that asks to be seen.
func (m Model) attentionText(text string) string {
	return lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextAccentAttention)).Render(text)
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
