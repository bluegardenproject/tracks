package tracksview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// draftActions are a draft's buttons: Enter starts it again.
var draftActions = []action{
	{actionStartDraft, "Start again", "s", 0},
	{actionDiscardDraft, "Discard", "d", 0},
}

// draftPromptLines is how much of a draft's prompt its details show.
const draftPromptLines = 6

// draftDetails draws draft t's details, width cells wide: why it failed
// and what was asked for.
func (m Model) draftDetails(t source.Track, width int) ([]string, []hit) {
	label := func(s string) string { return m.fg(theme.TextFaint).Render(pad(s, labelWidth)) }
	value := func(s string) string { return m.fg(theme.TextDefault).Render(s) }
	muted := func(s string) string { return m.fg(theme.TextMuted).Render(s) }
	wrap := func(c theme.Token, s string) []string {
		return strings.Split(m.fg(c).Width(max(1, width)).Render(s), "\n")
	}

	about := muted(t.Kind+"  ") + m.badge(t.Status)
	name := m.fg(theme.TextDefault).Bold(true).Render(cut(t.Shown(), max(1, width-lipgloss.Width(about)-1)))
	gap := max(1, width-lipgloss.Width(name)-lipgloss.Width(about))
	lines := []string{name + strings.Repeat(" ", gap) + about, ""}
	lines = append(lines, wrap(theme.StateDangerText, t.Draft.Error)...)
	lines = append(lines, "", label("Failed")+value(t.Created.Local().Format(createdLayout)))

	for i, r := range t.Repos {
		l := label("")
		if i == 0 {
			l = label("Repo")
		}
		lines = append(lines, l+value(r.Name))
	}
	engine := muted("the type's")
	if t.Engine != "" {
		engine = value(t.Engine)
		if t.Model != "" {
			engine += muted(" · " + t.Model)
		}
	}
	lines = append(lines, label("Engine")+engine, "")
	if prompt := strings.TrimSpace(t.Draft.Prompt); prompt != "" {
		shown := wrap(theme.TextMuted, prompt)
		if len(shown) > draftPromptLines {
			shown = append(shown[:draftPromptLines-1], muted("…"))
		}
		lines = append(append(lines, shown...), "")
	}
	row, hits := m.buttonRows(draftActions, t, width, len(lines))
	return append(lines, row...), hits
}

// draftAct starts draft t again in the New track form, or discards it.
func (m Model) draftAct(id actionID, t source.Track) tea.Cmd {
	start, discard := m.startDraft, m.discardDraft
	switch {
	case id == actionStartDraft && start != nil:
		return func() tea.Msg {
			if err := start(t.ID); err != nil {
				return doneMsg{err: fmt.Errorf("Couldn't open the New track form: %w", err)}
			}
			return doneMsg{reload: true}
		}
	case id == actionDiscardDraft && discard != nil:
		return func() tea.Msg {
			if err := discard(t.ID); err != nil {
				return doneMsg{err: fmt.Errorf("Couldn't discard the draft: %w", err)}
			}
			return doneMsg{ok: "Discarded the draft.", reload: true}
		}
	}
	return nil
}
