package tracksview

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// labelWidth is the width of the details' label column.
const labelWidth = 10

type actionID int

const (
	actionOpen actionID = iota
	actionEnd
	actionCopyPath
	actionCopySession
	actionOpenPR
	actionConfirmEnd
	actionCancel
)

// action is a details button; its key is underlined at hot.
type action struct {
	id    actionID
	label string
	key   string
	hot   int
}

var (
	actions = []action{
		{actionOpen, "Open", "o", 0},
		{actionEnd, "End", "e", 0},
		{actionCopyPath, "Copy path", "c", 0},
		{actionCopySession, "Copy session", "s", 5},
		{actionOpenPR, "Open PR", "p", 5},
	}
	confirmActions = []action{
		{actionConfirmEnd, "End track", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
)

// hit is where a button is drawn in the details panel.
type hit struct {
	id       actionID
	row, col int
	width    int
}

// details draws the selected track's details, width cells wide, and
// reports where its buttons are.
func (m Model) details(width int) ([]string, []hit) {
	t, ok := m.selectedTrack()
	if !ok {
		return nil, nil
	}
	label := func(s string) string { return m.fg(theme.TextFaint).Render(pad(s, labelWidth)) }
	value := func(s string) string { return m.fg(theme.TextDefault).Render(s) }
	muted := func(s string) string { return m.fg(theme.TextMuted).Render(s) }

	about := muted(t.Kind + " · " + t.Status)
	name := m.fg(theme.TextDefault).Bold(true).Render(t.Name)
	gap := max(1, width-lipgloss.Width(name)-lipgloss.Width(about))
	lines := []string{name + strings.Repeat(" ", gap) + about, "", label("ID") + value(strconv.Itoa(t.Number))}

	for i, r := range t.Repos {
		l := label("")
		if i == 0 {
			l = label("Repo")
		}
		line := l + value(r.Name)
		if r.Branch != "" {
			line += "  " + m.fg(theme.TextAccent).Render(r.Branch)
		}
		lines = append(lines, line, label("")+muted(shorten(r.Path, width-labelWidth)))
	}
	engine := muted("unknown")
	if t.Engine != "" {
		engine = value(t.Engine)
		if t.Model != "" {
			engine += muted(" · " + t.Model)
		}
	}
	session, pr := muted("none"), muted("none")
	if t.Session != "" {
		session = muted(shorten(t.Session, width-labelWidth))
	}
	if t.PR != nil {
		pr = value(fmt.Sprintf("#%d", t.PR.Number)) + muted(" "+t.PR.State)
	}
	lines = append(lines, label("Engine")+engine, label("Session")+session, label("PR")+pr, "")

	buttons := actions
	if m.station.confirming {
		lines = append(lines, m.fg(theme.StateWarningText).Render("End "+t.Name+"? Its window and agent close."), "")
		buttons = confirmActions
	}
	row, hits := m.buttonRows(buttons, t, width, len(lines))
	return append(lines, row...), hits
}

// fastTrackHeight is the Fast Track frame's height: 0 when the details'
// body, detailsLines long, wouldn't fit below it.
func (m Model) fastTrackHeight(detailsLines int) int {
	if m.contentHeight()-fastTrackRows < detailsLines+2 {
		return 0
	}
	return fastTrackRows
}

// fastTrack is the Fast Track frame's body, width cells wide.
func (m Model) fastTrack(width int) []string {
	about := m.fg(theme.TextMuted).Width(width).Render(
		"Templates that preset repos, type, engine and setup commands, so a new track only needs a slug and a prompt.")
	return append(strings.Split(about, "\n"), "", m.fg(theme.TextFaint).Render("Coming soon."))
}

// buttonRows lays buttons out from line top, wrapping at width.
func (m Model) buttonRows(buttons []action, t source.Track, width, top int) ([]string, []hit) {
	var rows []string
	var hits []hit
	line, col := "", 0
	for _, a := range buttons {
		button := widget.Button{Label: a.label, Hot: a.hot, Disabled: !m.enabled(a.id, t), Hover: a.id == m.station.hoverButton}
		if a.id == actionConfirmEnd {
			button.Kind = widget.ButtonDanger
		}
		b, w := button.View(m.palette), button.Width()
		if col > 0 && col+widget.ButtonGap+w > width {
			rows, line, col = append(rows, line), "", 0
		}
		if col > 0 {
			line += strings.Repeat(" ", widget.ButtonGap)
			col += widget.ButtonGap
		}
		if m.enabled(a.id, t) {
			hits = append(hits, hit{a.id, top + len(rows), col, w})
		}
		line += b
		col += w
	}
	return append(rows, line), hits
}

func (m Model) enabled(id actionID, t source.Track) bool {
	switch id {
	case actionCopyPath:
		return len(t.Repos) > 0 && t.Repos[0].Path != ""
	case actionCopySession:
		return t.Session != ""
	case actionOpenPR:
		return t.PR != nil && t.PR.URL != ""
	}
	return true
}

// buttonAt returns the details button drawn at cell x, y.
func (m Model) buttonAt(x, y int) (actionID, bool) {
	p := m.panes()
	if m.tab != tabStation || !p.details {
		return 0, false
	}
	left := p.detailsX + 2
	body, hits := m.details(p.detailsWidth - 4)
	top := m.contentTop() + 1 + m.fastTrackHeight(len(body))
	for _, h := range hits {
		if y == top+h.row && x >= left+h.col && x < left+h.col+h.width {
			return h.id, true
		}
	}
	return 0, false
}

// press runs a details action on the selected track.
func (m Model) press(id actionID) (Model, tea.Cmd, bool) {
	t, ok := m.selectedTrack()
	if !ok {
		return m, nil, false
	}
	if !m.enabled(id, t) {
		return m, nil, true
	}
	switch id {
	case actionEnd:
		m.station.confirming = true
		return m, nil, true
	case actionCancel:
		m.station.confirming = false
		return m, nil, true
	case actionCopyPath:
		m.station.notice = notice{text: "Copied the worktree path."}
		return m, tea.SetClipboard(t.Repos[0].Path), true
	case actionCopySession:
		m.station.notice = notice{text: "Copied the session ID."}
		return m, tea.SetClipboard(t.Session), true
	case actionConfirmEnd:
		m.station.confirming = false
		return m, m.act(actionEnd), true
	}
	return m, m.act(id), true
}

// act runs an action that leaves the Tracks window: switching to the
// track, ending it or opening its pull request.
func (m Model) act(id actionID) tea.Cmd {
	t, ok := m.selectedTrack()
	if !ok {
		return nil
	}
	run := func(f func() error, ok, failed string, reload bool) tea.Cmd {
		return func() tea.Msg {
			if err := f(); err != nil {
				return doneMsg{err: fmt.Errorf("%s: %w", failed, err)}
			}
			return doneMsg{ok: ok, reload: reload}
		}
	}
	switch {
	case id == actionOpen && m.open != nil:
		return run(func() error { return m.open(t.Number) }, "", "Couldn't switch", false)
	case id == actionEnd && m.end != nil:
		return run(func() error { return m.end(t.Number) }, "Ended "+t.Name+".", "Couldn't end "+t.Name, true)
	case id == actionOpenPR && m.openURL != nil && t.PR != nil:
		return run(func() error { return m.openURL(t.PR.URL) }, "", "Couldn't open the pull request", false)
	}
	return nil
}

// shorten writes the home directory as ~ and cuts s from the left to
// fit width.
func shorten(s string, width int) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(s, home) {
		s = "~" + strings.TrimPrefix(s, home)
	}
	r := []rune(s)
	if width < 2 || len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}
