package tracksview

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestArchiveAsksFirst(t *testing.T) {
	var unsaved []string
	var archived []string
	var forced []bool
	m := withEnded(Config{
		Unsaved: func(string) ([]string, error) { return unsaved, nil },
		Archive: func(id string, force bool) ([]string, error) {
			archived, forced = append(archived, id), append(forced, force)
			return nil, nil
		},
	})
	if view := plainView(m); !strings.Contains(view, " Archive ") {
		t.Fatalf("an ended track lacks Archive:\n%s", view)
	}

	m = settle(m, key('a'))
	view := plainView(m)
	for _, want := range []string{"Archive rate-bug? Its worktrees are", "removed; its branches stay.", " Archive ", " Cancel ", "y/Enter archive"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Archive should ask, missing %q:\n%s", want, view)
		}
	}
	if m = settle(m, key('n')); m.station.asking != nil || len(archived) != 0 {
		t.Fatal("cancelling archived anyway")
	}
	m = settle(settle(m, key('a')), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !slices.Equal(forced, []bool{false}) || m.station.notice.text != "Archived rate-bug." {
		t.Fatalf("confirmed: archived %v, hint row %q", forced, m.station.notice.text)
	}

	unsaved = []string{"web: 3 changed files"}
	m = settle(m, key('a'))
	if view := plainView(m); !strings.Contains(view, "web: 3 changed files") || !strings.Contains(view, "y remove and archive") {
		t.Fatalf("Archive should list the unsaved work:\n%s", view)
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter}); len(forced) != 1 || m.station.asking != nil {
		t.Fatalf("Enter should cancel removing unsaved work: archived %v", forced)
	}
	m = press(t, settle(m, key('a')), "Remove and archive")
	if !slices.Equal(forced, []bool{false, true}) {
		t.Errorf("Remove and archive: archived %v, want it forced", forced)
	}

	// old-fix is cleaned already: nothing to remove, nothing to ask.
	m = settle(settle(m, tea.KeyPressMsg{Code: tea.KeyDown}), key('a'))
	if m.station.asking != nil || !slices.Equal(archived, []string{"b", "b", "c"}) || m.station.notice.text != "Archived old-fix." {
		t.Errorf("a closed track: archived %v, asking %+v, hint row %q", archived, m.station.asking, m.station.notice.text)
	}
}

func TestOpenTracksCantBeArchived(t *testing.T) {
	calls := 0
	m := withEnded(Config{Archive: func(string, bool) ([]string, error) {
		calls++
		return nil, nil
	}})
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if view := plainView(m); strings.Contains(view, " Archive ") {
		t.Errorf("an open track offers Archive:\n%s", view)
	}
	if m = settle(m, key('a')); calls != 0 || m.station.asking != nil {
		t.Error("a archived an open track")
	}
}
