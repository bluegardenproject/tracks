package addtrack

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	escapes   = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
	down      = tea.KeyPressMsg{Code: tea.KeyDown}
	backspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func plain(m Model) string { return escapes.ReplaceAllString(m.render(), "") }

func TestPickingRepos(t *testing.T) {
	m := form("tracks", "ledger-live", "api")
	m = m.setFocus(ctlRepos)
	if !strings.Contains(plain(m), "[ "+selectRepos) {
		t.Fatal("Repos should start as the Select repos field")
	}
	m, _ = send(m, enter)
	m, _ = send(m, append(typed("le"), space, esc)...)
	if m.picker != nil || !slices.Equal(m.pickedRepos(), []string{"ledger-live"}) {
		t.Fatalf("filtering, ticking, Esc: picker open %v, picked %v", m.picker != nil, m.pickedRepos())
	}
	m, _ = send(m, space, space, enter)
	if got := m.pickedRepos(); !slices.Equal(got, []string{"tracks", "ledger-live"}) {
		t.Fatalf("reopening and ticking tracks: picked %v", got)
	}
	if view := plain(m); !strings.Contains(view, " tracks ✕   ledger-live ✕ ") || !strings.Contains(view, "[ "+selectRepos) {
		t.Errorf("the picked repos should show in a row under the selector:\n%s", view)
	}

	m, _ = send(m, right, backspace)
	if got := m.pickedRepos(); !slices.Equal(got, []string{"ledger-live"}) || m.item != 1 {
		t.Fatalf("Backspace on tracks: picked %v, cursor %d", got, m.item)
	}
	m, _ = send(m, enter)
	if len(m.pickedRepos()) != 0 || m.item != 0 {
		t.Errorf("Enter on the last repo: picked %v, cursor %d", m.pickedRepos(), m.item)
	}
}

func TestReviewPicksOneRepo(t *testing.T) {
	m := form("tracks", "ledger-live")
	m = m.setKind(Review)
	m = m.setFocus(ctlRepo)
	if !strings.Contains(plain(m), "[ tracks") {
		t.Fatal("Review's Repo should show the first repo")
	}
	m, _ = send(m, enter)
	if m.picker == nil || m.picker.Ticked != nil {
		t.Fatal("Enter on Repo should open a picker to choose one")
	}
	m, _ = send(m, down, enter)
	if m.picker != nil || m.repo != 1 || !strings.Contains(plain(m), "[ ledger-live") {
		t.Errorf("choosing ledger-live: picker open %v, repo %d", m.picker != nil, m.repo)
	}
}
