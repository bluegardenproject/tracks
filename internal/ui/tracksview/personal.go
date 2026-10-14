package tracksview

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/commentstyle"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

const personalAbout = "Tracks' agents answer review comments under your name. Learn how you write from your PR comments of the last 60 days, and they'll answer the way you do. Comments they wrote themselves are left out."

// spinner turns in the log's title while learning runs.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// maxLog is how many log lines are kept.
const maxLog = 500

// personalState is the Personalisation section's state.
type personalState struct {
	status    commentstyle.Status
	statusErr error
	running   bool
	failed    bool
	log       []logLine
	frame     int  // the spinner's
	hover     bool // the mouse is on the button
}

// logLine is a line of the log and when it came.
type logLine struct {
	at time.Time
	commentstyle.Line
}

type (
	styleStatusMsg struct {
		status commentstyle.Status
		err    error
	}
	styleEvent struct {
		line   *commentstyle.Line // nil when done
		status commentstyle.Status
		err    error
		events <-chan styleEvent
	}
	styleTickMsg struct{}
)

// loadStyleStatus reads whether the comment style is learned.
func (m Model) loadStyleStatus() tea.Cmd {
	if m.styleStatus == nil {
		return nil
	}
	read := m.styleStatus
	return func() tea.Msg {
		st, err := read()
		return styleStatusMsg{st, err}
	}
}

// startLearning learns the comment style, logging each step.
func (m Model) startLearning() (Model, tea.Cmd) {
	p := &m.settings.personal
	if m.learnStyle == nil || p.running {
		return m, nil
	}
	p.running, p.failed, p.log = true, false, nil
	events := make(chan styleEvent, 64)
	learn := m.learnStyle
	go func() {
		st, err := learn(func(s string) {
			l := commentstyle.ParseLine(s)
			events <- styleEvent{line: &l, events: events}
		})
		events <- styleEvent{status: st, err: err, events: events}
	}()
	return m, tea.Batch(nextStyleEvent(events), styleTick())
}

func nextStyleEvent(events <-chan styleEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func styleTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return styleTickMsg{} })
}

func (m Model) styleEvent(e styleEvent) (Model, tea.Cmd) {
	p := &m.settings.personal
	if e.line != nil {
		p.log = append(p.log, logLine{time.Now(), *e.line})
		if len(p.log) > maxLog {
			p.log = p.log[len(p.log)-maxLog:]
		}
		return m, nextStyleEvent(e.events)
	}
	p.running = false
	if e.err != nil {
		p.failed = true
		if len(p.log) == 0 || p.log[len(p.log)-1].Kind != commentstyle.Fail {
			p.log = append(p.log, logLine{time.Now(), commentstyle.Line{Kind: commentstyle.Fail, Text: e.err.Error()}})
		}
		return m, nil
	}
	p.status, p.statusErr = e.status, nil
	m.settings.notice = notice{text: "Learned your comment style."}
	return m, nil
}

func (m Model) styleTicked() (Model, tea.Cmd) {
	p := &m.settings.personal
	if !p.running {
		return m, nil
	}
	p.frame = (p.frame + 1) % len(spinner)
	return m, styleTick()
}

// styleButton is the section's button: learn, or learn again.
func (m Model) styleButton() widget.Button {
	p := m.settings.personal
	label := "Learn my comment style"
	if p.status.Learned {
		label = "Relearn"
	}
	b := widget.NewButton(label, widget.ButtonAccent)
	b.Hover = p.hover
	b.Disabled = p.running || m.learnStyle == nil
	return b
}

// personalTop is the section's lines above the log, and the row of the
// button among them.
func (m Model) personalTop(width int) (lines []string, buttonRow int) {
	p := m.settings.personal
	lines = append(lines, m.fg(theme.TextDefault).Bold(true).Render("PR comment style"))
	lines = append(lines, strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(personalAbout), "\n")...)
	lines = append(lines, "")
	switch {
	case p.statusErr != nil:
		lines = append(lines, m.fg(theme.StateDangerText).Render(cut("Couldn't read the guide: "+p.statusErr.Error(), width)))
	case !p.status.Learned:
		lines = append(lines, m.fg(theme.TextMuted).Render("Not learned yet."))
	case p.status.At.IsZero():
		lines = append(lines, m.fg(theme.TextDefault).Render("Written by hand."))
	default:
		lines = append(lines, m.fg(theme.StateSuccessText).Render("● ")+m.fg(theme.TextDefault).Render(
			fmt.Sprintf("Learned %s from %d comments.", p.status.At.Local().Format("2 Jan 2006, 15:04"), p.status.Comments)))
	}
	if m.stylePath != "" {
		lines = append(lines, m.fg(theme.TextFaint).Render(cut(m.stylePath, width)))
	}
	lines = append(lines, "")
	buttonRow = len(lines)
	return append(lines, m.styleButton().View(m.palette), ""), buttonRow
}

func (m Model) personal(width, height int) []string {
	top, _ := m.personalTop(width)
	return append(top, m.logBox(width, max(4, height-len(top)))...)
}

// logBox draws the log as a terminal: a framed panel, a line per step
// with its time and mark, the newest at the bottom, and a prompt.
func (m Model) logBox(width, height int) []string {
	p := m.settings.personal
	inner := max(10, width-4)
	bg := lipgloss.NewStyle().Background(m.palette.Color(theme.BgSurface))
	on := func(token theme.Token) lipgloss.Style { return bg.Foreground(m.palette.Color(token)) }
	border := m.fg(theme.BorderDefault)

	state, stateColour := "idle", theme.TextFaint
	switch {
	case p.running:
		state, stateColour = spinner[p.frame]+" learning", theme.StateInfoText
	case p.failed:
		state, stateColour = "✗ failed", theme.StateDangerText
	case len(p.log) > 0:
		state, stateColour = "✓ done", theme.StateSuccessText
	}
	title := " log "
	fill := max(0, width-2-lipgloss.Width(title)-lipgloss.Width(state)-3)
	lines := []string{border.Render("╭─") + m.fg(theme.TextMuted).Render(title) + border.Render(strings.Repeat("─", fill)+" ") +
		m.fg(stateColour).Render(state) + border.Render(" ╮")}

	rows := max(1, height-2)
	var body []string
	if len(p.log) == 0 {
		body = append(body, on(theme.TextFaint).Render(pad("Ready. Tracks reads your comments with gh, then has your default engine write the guide.", inner)))
	}
	for _, l := range p.log[max(0, len(p.log)-rows+1):] {
		mark, text := markColour(l.Kind), theme.TextDefault
		switch l.Kind {
		case commentstyle.Note:
			text = theme.TextMuted
		case commentstyle.Fail:
			text = theme.StateDangerText
		}
		stamp := on(theme.TextFaint).Render(l.at.Format("15:04:05") + " ")
		body = append(body, stamp+on(mark).Render(string(l.Kind)+" ")+on(text).Render(pad(cut(l.Text, inner-11), inner-11)))
	}
	cursor := " "
	if p.running && p.frame%6 < 3 {
		cursor = "▍"
	}
	prompt := on(theme.TextAccent).Render("❯ ") + on(theme.TextDefault).Render(pad(cursor, inner-2))
	for len(body) < rows-1 {
		body = append([]string{on(theme.TextDefault).Render(strings.Repeat(" ", inner))}, body...)
	}
	body = append(body[max(0, len(body)-(rows-1)):], prompt)
	for _, b := range body {
		lines = append(lines, border.Render("│")+bg.Render(" ")+b+bg.Render(" ")+border.Render("│"))
	}
	return append(lines, border.Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
}

// markColour is the colour of a log line's mark.
func markColour(k rune) theme.Token {
	switch k {
	case commentstyle.Request:
		return theme.TextAccent
	case commentstyle.Answer:
		return theme.StateInfoText
	case commentstyle.Fail:
		return theme.StateDangerText
	case commentstyle.Done:
		return theme.StateSuccessText
	}
	return theme.TextFaint
}

// onStyleButton reports whether section cell bx, by is on the button.
func (m Model) onStyleButton(bx, by int) bool {
	_, row := m.personalTop(m.sectionWidth())
	return by == row && bx >= 0 && bx < m.styleButton().Width()
}
