package dashboard

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

// key builds the tea.KeyMsg for a single rune, matching how bubbletea
// delivers ordinary keystrokes.
func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// Removing a track is irreversible, so `x` must only arm a confirmation
// — never call the daemon on the first press.
func TestRemoveKeyArmsConfirmation(t *testing.T) {
	m := makeModel(3, 120, 40)
	m.tracks[1].Status = state.StatusDone
	m.cursor = 1

	m.Update(key('x'))

	if m.confirm == nil {
		t.Fatal("x on a finished track did not arm a confirmation")
	}
	if !strings.Contains(m.confirm.title, "Remove") {
		t.Errorf("confirm title = %q, want it to mention Remove", m.confirm.title)
	}
	if !strings.Contains(m.confirm.title, "track-001") {
		t.Errorf("confirm title = %q, want it to name the track", m.confirm.title)
	}
}

// A running track has nothing to remove, so `x` must stay inert there.
func TestRemoveKeyIgnoresLiveTrack(t *testing.T) {
	m := makeModel(3, 120, 40) // all running
	m.cursor = 0

	m.Update(key('x'))

	if m.confirm != nil {
		t.Errorf("x on a running track armed %q", m.confirm.title)
	}
}

// While a confirmation is pending the dashboard is modal: n cancels it,
// and unrelated keys (here: cursor movement) do nothing.
func TestConfirmationIsModal(t *testing.T) {
	m := makeModel(3, 120, 40)
	m.tracks[1].Status = state.StatusDone
	m.cursor = 1
	m.Update(key('x'))

	m.Update(key('j')) // would normally move the cursor down
	if m.cursor != 1 {
		t.Errorf("cursor moved to %d while a confirmation was pending", m.cursor)
	}
	if m.confirm == nil {
		t.Fatal("an unrelated key dismissed the confirmation")
	}

	m.Update(key('n'))
	if m.confirm != nil {
		t.Error("n did not cancel the confirmation")
	}
}

// Esc is the other way out — same effect as n.
func TestConfirmationEscCancels(t *testing.T) {
	m := makeModel(2, 120, 40)
	m.tracks[0].Status = state.StatusErrored
	m.Update(key('x'))
	if m.confirm == nil {
		t.Fatal("x did not arm a confirmation")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirm != nil {
		t.Error("esc did not cancel the confirmation")
	}
}

// `X` (clear completed) is gated too, and says how many records go.
func TestClearCompletedArmsConfirmation(t *testing.T) {
	m := makeModel(4, 120, 40)
	m.tracks[0].Status = state.StatusDone
	m.tracks[1].Status = state.StatusPRMerged
	m.tracks[2].Status = state.StatusInterrupted // kept by prune

	m.Update(key('X'))

	if m.confirm == nil {
		t.Fatal("X did not arm a confirmation")
	}
	if !strings.Contains(m.confirm.title, "2") {
		t.Errorf("confirm title = %q, want the count of 2 completed tracks", m.confirm.title)
	}
}

// Nothing to clear: no modal, just a status line.
func TestClearCompletedWithNothingToDo(t *testing.T) {
	m := makeModel(2, 120, 40) // both running

	m.Update(key('X'))

	if m.confirm != nil {
		t.Errorf("X armed %q with no completed tracks", m.confirm.title)
	}
	if m.statusMsg == "" {
		t.Error("X with nothing to clear left no explanation")
	}
}

// The confirmation is rendered inside the frame, and the frame must
// still respect the terminal height (bubbletea garbles overflow).
func TestConfirmationRendersWithinHeight(t *testing.T) {
	for _, h := range []int{10, 24, 40} {
		m := makeModel(20, 120, h)
		m.tracks[3].Status = state.StatusDone
		m.cursor = 3
		m.Update(key('x'))
		out := m.View()
		if got := lineCount(out); got > h {
			t.Errorf("height %d: view is %d lines with a confirmation open", h, got)
		}
		if h >= 24 && !strings.Contains(out, "Remove") {
			t.Errorf("height %d: confirmation not visible in the frame", h)
		}
	}
}

// The footer names the actions by the labels the keys actually perform.
func TestFooterActionLabels(t *testing.T) {
	m := makeModel(1, 160, 40)
	out := m.View()
	for _, want := range []string{"d close", "x remove", "K kill"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer is missing %q", want)
		}
	}
	if strings.Contains(out, "x forget") || strings.Contains(out, "d end") {
		t.Error("footer still shows an old action label")
	}
}
