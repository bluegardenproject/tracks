package tracksview

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

var (
	confirmDerail = []action{
		{actionConfirmDerail, "Derail", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
	confirmDerailAnyway = []action{
		{actionConfirmDerail, "Derail anyway", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
)

// derailQuestion is what Station asks before derailing t: the work in q
// it would lose, then whether to delete t for good.
func derailQuestion(t source.Track, q question) ([]string, []action) {
	ask := append(slices.Clone(q.lines), "Derail "+t.Name+"? Its worktrees, branches and record are deleted for good.")
	if pr, ok := t.MainPR(); ok && (pr.State == string(track.PROpen) || pr.State == string(track.PRDraft)) {
		ask = append(ask, "Its PR #"+strconv.Itoa(pr.Number)+" stays open on GitHub.")
	}
	if len(q.lines) > 0 {
		return ask, confirmDerailAnyway
	}
	return ask, confirmDerail
}

// checkDerail looks for the work derailing t would lose, and then asks.
func (m Model) checkDerail(t source.Track) (Model, tea.Cmd) {
	if m.lostFn == nil {
		return m, nil
	}
	lost := m.lostFn
	m.station.notice = notice{text: "Checking what derailing " + t.Name + " would lose…", busy: true}
	return m, func() tea.Msg {
		found, err := lost(t.ID)
		return checkedMsg{id: t.ID, name: t.Name, kind: askDerail, unsaved: found, err: err}
	}
}

// derail deletes q's track, anyway when q listed work it would lose.
func (m Model) derail(q question) tea.Cmd {
	if m.derailFn == nil {
		return nil
	}
	derail := m.derailFn
	return func() tea.Msg {
		found, err := derail(q.id, len(q.lines) > 0)
		return derailedMsg{id: q.id, name: q.name, kind: askDerail, unsaved: found, err: err}
	}
}

func (m Model) derailed(msg derailedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.station.notice = notice{text: failure("Couldn't derail "+msg.name, msg.err), err: true}
	case len(msg.unsaved) > 0:
		// Work that appeared after the check: ask again.
		m = m.ask(question{id: msg.id, name: msg.name, kind: askDerail, lines: msg.unsaved})
	default:
		m.station.notice = notice{text: "Derailed " + msg.name + "."}
	}
	return m, m.loadTracks()
}
