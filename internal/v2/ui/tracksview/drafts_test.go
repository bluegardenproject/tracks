package tracksview

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// withDraft is Station with an open track and a draft below it,
// selected.
func withDraft(c Config) Model {
	c.Version, c.Theme = "test", theme.Default()
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "open-one", Kind: "work", Status: track.Active},
		{ID: "d", Title: "Fix the login", Kind: "work", Status: track.Draft, Engine: "Claude Code", Model: "opus",
			Repos: []source.Repo{{Name: "web"}}, Created: time.Date(2026, 9, 30, 17, 0, 0, 0, time.Local),
			Draft: &source.Draft{Error: "Add Cursor on the Engines tab.", Prompt: "Fix the login\nand the logout"}},
	}
	return update(New(c), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks}, tea.KeyPressMsg{Code: tea.KeyDown})
}

func TestDraftRow(t *testing.T) {
	var started, discarded []string
	m := withDraft(Config{
		StartDraft:   func(id string) error { started = append(started, id); return nil },
		DiscardDraft: func(id string) error { discarded = append(discarded, id); return nil },
	})
	view := plainView(m)
	for _, want := range []string{"Fix the login", " draft ", "Add Cursor on the Engines tab.", "and the logout", " Start again ", " Discard ", "Enter start again"} {
		if !strings.Contains(view, want) {
			t.Errorf("the draft's row and details should show %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{" Open ", " Resume ", "Session"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("a draft has no %q:\n%s", unwanted, view)
		}
	}

	m = settle(m, enterKey)
	if !slices.Equal(started, []string{"d"}) {
		t.Errorf("Enter started %q, want the draft", started)
	}
	m = settle(m, key('d'))
	if !slices.Equal(discarded, []string{"d"}) || m.station.notice.text != "Discarded the draft." {
		t.Errorf("d discarded %q, hint row %q", discarded, m.station.notice.text)
	}
}

func TestDraftDiscardFails(t *testing.T) {
	m := withDraft(Config{DiscardDraft: func(string) error { return errors.New("That draft is gone.") }})
	m = press(t, m, "Discard")
	if got := m.station.notice.text; got != "Couldn't discard the draft: That draft is gone." {
		t.Errorf("hint row %q", got)
	}
}
