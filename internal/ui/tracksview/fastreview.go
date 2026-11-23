package tracksview

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// FastReviewFunc opens a Review track on the pull request at link, in
// the background, telling progress each slow step, and returns the
// name it's shown under.
type FastReviewFunc func(link string, progress func(string)) (string, error)

// Station's Fast Track frame holds Fast Review: a pull request link
// pasted into its input, then Enter or its Review button, opens a
// Review track on it.
const (
	// fastLabel is the button's label. The input, in brackets in the
	// Fast Track frame's colour, takes the width the button leaves.
	fastLabel = "Review"
	// fastLinkLimit bounds what the input takes.
	fastLinkLimit = 512
)

// fastKeys are the hints while the input has focus.
var fastKeys = []keyHelp{{"Enter", "open the review"}, {"Ctrl+V", "paste"}, {"Esc", "leave"}}

func newFastInput() textinput.Model {
	return widget.NewInput(fastLinkLimit, "Pull request link for review")
}

// fastButton is the Review button, as the Details' buttons are drawn:
// faint until there's a link to review.
func (m Model) fastButton() widget.Button {
	return widget.Button{Label: fastLabel, Hot: -1, Disabled: strings.TrimSpace(m.station.fast.Value()) == "",
		Hover: m.station.hoverFast}
}

// fastBoxes are the input's width, brackets included, and the button's,
// width cells wide in all with the gap between them.
func fastBoxes(width int) (input, button int) {
	button = widget.Button{Label: fastLabel}.Width()
	return max(4, width-button-widget.ButtonGap), button
}

// fastTrack is the Fast Track frame's body, width cells wide: the input
// and the Review button beside it.
func (m Model) fastTrack(width int) []string {
	inputW, _ := fastBoxes(width)
	return []string{widget.Input(m.palette, m.station.fast, inputW-2, theme.BorderAccent) +
		strings.Repeat(" ", widget.ButtonGap) + m.fastButton().View(m.palette)}
}

// fastShown reports whether the Fast Track frame is drawn.
func (m Model) fastShown() bool {
	p := m.panes()
	if m.tab != tabStation || !p.details || m.station.err != nil {
		return false
	}
	if len(m.station.tracks) == 0 {
		return true
	}
	body, _ := m.details(p.detailsWidth - 4)
	return m.fastTrackHeight(len(body)) > 0
}

// fastAt reports whether cell x, y is on Fast Review's input or on
// its button.
func (m Model) fastAt(x, y int) (input, button bool) {
	if !m.fastShown() {
		return false, false
	}
	p := m.panes()
	left, top := p.detailsX+2, m.contentTop()+1
	if y != top {
		return false, false
	}
	inputW, buttonW := fastBoxes(p.detailsWidth - 4)
	right := left + inputW + widget.ButtonGap
	return x >= left && x < left+inputW, x >= right && x < right+buttonW
}

func (m Model) onFastInput(x, y int) bool  { input, _ := m.fastAt(x, y); return input }
func (m Model) onFastButton(x, y int) bool { _, button := m.fastAt(x, y); return button }

// sizeFast tells Fast Review's input its width, which it scrolls to
// its cursor by as text comes in; the cursor takes a cell.
func (m Model) sizeFast() Model {
	inputW, _ := fastBoxes(m.panes().detailsWidth - 4)
	m.station.fast.SetWidth(max(1, inputW-3))
	return m
}

// focusFast gives Fast Review's input the keys.
func (m Model) focusFast() (Model, tea.Cmd) {
	if m.fastReview == nil || !m.fastShown() {
		return m, nil
	}
	m.station.asking = nil
	return m, m.station.fast.Focus()
}

// fastKey handles a key while Fast Review's input has focus.
func (m Model) fastKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.station.fast.Blur()
		return m, nil
	case "tab", "shift+tab":
		m.station.fast.Blur()
		next, cmd := m.key(msg)
		return next.(Model), cmd
	case "enter":
		return m.startFastReview()
	}
	m = m.sizeFast()
	var cmd tea.Cmd
	m.station.fast, cmd = m.station.fast.Update(msg)
	return m, cmd
}

// fastPaste puts what's pasted on Station into Fast Review's input: a
// link pasted anywhere on Station replaces what's there.
func (m Model) fastPaste(msg tea.PasteMsg) (Model, tea.Cmd) {
	if m.fastReview == nil || !m.fastShown() {
		return m, nil
	}
	if !m.station.fast.Focused() {
		m.station.fast.SetValue("")
	}
	next, focus := m.sizeFast().focusFast()
	var cmd tea.Cmd
	next.station.fast, cmd = next.station.fast.Update(tea.PasteMsg{Content: strings.TrimSpace(msg.Content)})
	return next, tea.Batch(focus, cmd)
}

// startFastReview opens a Review track on the input's link, and clears
// the input for the next one.
func (m Model) startFastReview() (Model, tea.Cmd) {
	link := strings.TrimSpace(m.station.fast.Value())
	if link == "" || m.fastReview == nil {
		return m, nil
	}
	m.station.fast.SetValue("")
	m.station.fast.Blur()
	open := m.fastReview
	events := make(chan progressEvent, 16)
	go func() {
		name, err := open(link, func(s string) { events <- progressEvent{progress: s, events: events} })
		events <- progressEvent{done: true, ok: "Opened " + name + ".", failed: "Couldn't open a Fast Review",
			err: err, events: events}
	}()
	m.station.notice = notice{text: "Opening a Fast Review…", busy: true}
	return m, nextProgress(events)
}
