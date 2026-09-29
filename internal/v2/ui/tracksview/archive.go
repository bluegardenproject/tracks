package tracksview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

var (
	confirmArchive = []action{
		{actionConfirmArchive, "Archive", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
	confirmArchiveAnyway = []action{
		{actionConfirmArchive, "Remove and archive", "y", -1},
		{actionCancel, "Cancel", "n", -1},
	}
)

// archiveQuestion is what Station asks before archiving t: the unsaved
// work in q, or whether to remove its worktrees.
func archiveQuestion(t source.Track, q question) ([]string, []action) {
	if len(q.lines) > 0 {
		return q.lines, confirmArchiveAnyway
	}
	return []string{"Archive " + t.Name + "? Its worktrees are removed; its branches stay."}, confirmArchive
}

// checkArchive archives t, asking first when it has worktrees to
// remove.
func (m Model) checkArchive(t source.Track) (Model, tea.Cmd) {
	if t.Cleanable {
		return m.checkUnsaved(t, askArchive)
	}
	return m, m.archive(question{id: t.ID, name: t.Name, kind: askArchive})
}

// archive archives q's track, removing its worktrees anyway when q
// listed unsaved work.
func (m Model) archive(q question) tea.Cmd {
	if m.archiveFn == nil {
		return nil
	}
	archive := m.archiveFn
	return func() tea.Msg {
		found, err := archive(q.id, len(q.lines) > 0)
		return archivedMsg{id: q.id, name: q.name, kind: askArchive, unsaved: found, err: err}
	}
}

func (m Model) archived(msg archivedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.station.notice = notice{text: failure("Couldn't archive "+msg.name, msg.err), err: true}
	case len(msg.unsaved) > 0:
		// Work that appeared after the check: ask again.
		m = m.ask(question{id: msg.id, name: msg.name, kind: askArchive, lines: msg.unsaved})
	default:
		m.station.notice = notice{text: "Archived " + msg.name + "."}
	}
	return m, m.loadTracks(false)
}
