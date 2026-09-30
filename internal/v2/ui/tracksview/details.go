package tracksview

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const (
	// labelWidth is the width of the details' label column.
	labelWidth = 10
	// createdLayout is how the details show when a track was created.
	createdLayout = "2006-01-02 15:04"
)

type actionID int

const (
	actionOpen actionID = iota
	actionEnd
	actionResume
	actionArchive
	actionUnarchive
	actionDerail
	actionPromote
	actionRestart
	actionCopyPath
	actionCopySession
	actionOpenPR
	actionConfirmEnd
	actionConfirmRecreate
	actionConfirmArchive
	actionConfirmDerail
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
	// openActions are an open track's buttons, endedActions an ended
	// one's.
	openActions = []action{
		{actionOpen, "Open", "o", 0},
		{actionEnd, "End", "e", 0},
		{actionCopyPath, "Copy path", "c", 0},
		{actionCopySession, "Copy session", "s", 5},
		{actionOpenPR, "Open PR", "p", 5},
	}
	endedActions = append([]action{
		{actionResume, "Resume", "r", 0},
		{actionArchive, "Archive", "a", 0},
		{actionDerail, "Derail", "d", 0},
	}, openActions[2:]...)
	// archivedActions are an archived track's.
	archivedActions = append([]action{{actionUnarchive, "Unarchive", "u", 0}, {actionDerail, "Derail", "d", 0}}, openActions[2:]...)
	confirmEnd      = []action{
		{actionConfirmEnd, "End track", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
)

// confirmRecreate are the buttons for n missing worktrees.
func confirmRecreate(n int) []action {
	label := "Re-create worktree"
	if n > 1 {
		label += "s"
	}
	return []action{{actionConfirmRecreate, label, "y", -1}, {actionCancel, "Cancel", "n", -1}}
}

// promoteAction is an Ask or Plan track's, after Open or Resume;
// restartAction an open track's whose agent exited, after End.
var (
	promoteAction = action{actionPromote, "Promote", "m", 3}
	restartAction = action{actionRestart, "Restart", "r", 0}
)

// actionsFor are t's buttons.
func actionsFor(t source.Track) []action {
	if t.Archived {
		return archivedActions
	}
	promotable := t.Kind == string(track.Ask) || t.Kind == string(track.Plan)
	if !t.Open() {
		if promotable {
			return slices.Insert(slices.Clone(endedActions), 1, promoteAction)
		}
		return endedActions
	}
	actions := openActions
	if promotable {
		actions = slices.Insert(slices.Clone(actions), 2, promoteAction)
	}
	if t.Status == track.Exited || t.Status == track.Error {
		actions = slices.Insert(slices.Clone(actions), 2, restartAction)
	}
	return actions
}

// mainAction is what Enter and a double click do to t.
func mainAction(t source.Track) actionID {
	switch {
	case t.Open():
		return actionOpen
	case t.Archived:
		return actionUnarchive
	}
	return actionResume
}

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

	about := muted(t.Kind+"  ") + m.statusBadges(t, lipgloss.NewStyle())
	name := m.fg(theme.TextDefault).Bold(true).Render(t.Shown())
	gap := max(1, width-lipgloss.Width(name)-lipgloss.Width(about))
	number := muted("none")
	if t.Number > 0 {
		number = value(strconv.Itoa(t.Number))
	}
	created := muted("unknown")
	if !t.Created.IsZero() {
		created = value(t.Created.Local().Format(createdLayout))
	}
	lines := []string{name + strings.Repeat(" ", gap) + about, "", label("Slug") + value(t.Name), label("ID") + number, label("Created") + created}

	for i, r := range t.Repos {
		l := label("")
		if i == 0 {
			l = label("Repo")
		}
		line := l + value(r.Name)
		if r.Branch != "" {
			line += "  " + m.fg(theme.TextAccent).Render(r.Branch)
		}
		path := muted(shorten(r.Path, width-labelWidth))
		if r.Removed {
			path = muted("removed")
		}
		lines = append(lines, line, label("")+path)
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
	if len(t.PRs) > 0 {
		pr = prList(t.PRs, value, muted)
	}
	lines = append(lines, label("Engine")+engine, label("Session")+session, label("PR")+pr, "")

	buttons := actionsFor(t)
	if q := m.station.asking; q != nil {
		var ask []string
		switch {
		case q.kind == askEnd:
			ask, buttons = []string{"End " + t.Name + "? Its window and agent close."}, confirmEnd
		case q.kind == askRecreate:
			heading := "Worktree couldn't be found"
			if len(q.lines) > 1 {
				heading = "Worktrees couldn't be found"
			}
			ask, buttons = append([]string{heading}, q.lines...), confirmRecreate(len(q.lines))
			if t.Removed() {
				ask = append(ask, "A deleted branch comes back from origin, or starts again from the base.")
			}
		case q.kind == askArchive:
			ask, buttons = archiveQuestion(t, *q)
		case q.kind == askDerail:
			ask, buttons = derailQuestion(t, *q)
		}
		warn := m.fg(theme.StateWarningText).Width(width)
		for _, a := range ask {
			lines = append(lines, strings.Split(warn.Render(a), "\n")...)
		}
		lines = append(lines, "")
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
		if a.id == actionConfirmEnd || a.id == actionConfirmArchive || a.id == actionConfirmDerail {
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
		_, ok := t.MainPR()
		return ok
	case actionResume:
		return t.Status != track.Closed
	case actionArchive:
		return !t.Open() && !t.Archived
	case actionDerail:
		return !t.Open()
	case actionPromote:
		return len(t.Repos) > 0
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
		m.station.asking = &question{id: t.ID, name: t.Name, kind: askEnd}
		return m, nil, true
	case actionCancel:
		m.station.asking = nil
		return m, nil, true
	case actionResume:
		next, cmd := m.startResume(t.ID, t.Name, false)
		return next, cmd, true
	case actionArchive:
		next, cmd := m.checkArchive(t)
		return next, cmd, true
	case actionDerail:
		next, cmd := m.checkDerail(t)
		return next, cmd, true
	case actionPromote:
		next, cmd := m.startPromote(t.ID, t.Name)
		return next, cmd, true
	case actionRestart:
		next, cmd := m.startRestart(t.ID, t.Name)
		return next, cmd, true
	case actionConfirmEnd, actionConfirmRecreate, actionConfirmArchive, actionConfirmDerail:
		q := m.station.asking
		m.station.asking = nil
		if q == nil {
			return m, nil, true
		}
		next, cmd := m.confirm(*q)
		return next, cmd, true
	case actionCopyPath:
		m.station.notice = notice{text: "Copied the worktree path."}
		return m, tea.SetClipboard(t.Repos[0].Path), true
	case actionCopySession:
		m.station.notice = notice{text: "Copied the session ID."}
		return m, tea.SetClipboard(t.Session), true
	}
	return m, m.act(id), true
}

// act runs an action that leaves the Tracks window or reloads it:
// switching to the track, ending or unarchiving it, or opening its pull
// request.
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
	case id == actionUnarchive && m.unarchive != nil:
		return run(func() error { return m.unarchive(t.ID) }, "Unarchived "+t.Name+".", "Couldn't unarchive "+t.Name, true)
	case id == actionOpenPR && m.openURL != nil:
		if pr, ok := t.MainPR(); ok {
			return run(func() error { return m.openURL(pr.URL) }, "", "Couldn't open the pull request", false)
		}
	}
	return nil
}

// prList is prs as "#12 open", with each repo's name when they're in
// several repos.
func prList(prs []source.PR, value, muted func(string) string) string {
	several := false
	for _, p := range prs {
		several = several || p.Repo != prs[0].Repo
	}
	parts := make([]string, len(prs))
	for i, p := range prs {
		name := ""
		if several {
			_, name, _ = strings.Cut(p.Repo, "/")
		}
		parts[i] = value(fmt.Sprintf("%s#%d", name, p.Number)) + muted(" "+p.State)
	}
	return strings.Join(parts, muted(", "))
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
