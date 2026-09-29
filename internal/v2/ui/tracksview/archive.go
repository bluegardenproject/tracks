package tracksview

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

var (
	confirmArchive = []action{
		{actionConfirmArchive, "Archive", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
	confirmArchiveAnyway = []action{
		{actionConfirmArchive, "Archive anyway", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
)

// archiveQuestion is what Station asks before archiving t: the work in
// q it would lose, then whether to remove its worktrees and branches.
func archiveQuestion(t source.Track, q question) ([]string, []action) {
	ask := append(slices.Clone(q.lines), "Archive "+t.Name+"? Its worktrees and local branches are removed; what was pushed stays.")
	if len(q.lines) > 0 {
		return ask, confirmArchiveAnyway
	}
	return ask, confirmArchive
}

// checkArchive archives t, asking first when it has worktrees or
// branches to remove.
func (m Model) checkArchive(t source.Track) (Model, tea.Cmd) {
	if t.Removable {
		return m.checkLost(t, askArchive, "Checking what archiving "+t.Name+" would lose…")
	}
	return m, m.archive(question{id: t.ID, name: t.Name, kind: askArchive})
}

// archive archives q's track, removing its worktrees and branches
// anyway when q listed work they would lose.
func (m Model) archive(q question) tea.Cmd {
	if m.archiveFn == nil {
		return nil
	}
	archive := m.archiveFn
	return func() tea.Msg {
		found, err := archive(q.id, len(q.lines) > 0)
		return archivedMsg{id: q.id, name: q.name, kind: askArchive, lost: found, err: err}
	}
}

func (m Model) archived(msg archivedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.station.notice = notice{text: failure("Couldn't archive "+msg.name, msg.err), err: true}
	case len(msg.lost) > 0:
		// Work that appeared after the check: ask again.
		m = m.ask(question{id: msg.id, name: msg.name, kind: askArchive, lines: msg.lost})
	default:
		m.station.notice = notice{text: "Archived " + msg.name + "."}
	}
	return m, m.loadTracks()
}
