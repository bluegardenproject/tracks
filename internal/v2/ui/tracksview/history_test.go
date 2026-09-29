package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

type fakeHistory struct{ saved settings.History }

func (f *fakeHistory) Load() (settings.History, error) { return f.saved, nil }

func (f *fakeHistory) Save(h settings.History) error {
	f.saved = h
	return nil
}

func TestTracksHistory(t *testing.T) {
	engines := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{Model: "opus"}}}
	history := &fakeHistory{}
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), Engines: engines,
		TrackTypes: &fakeTypes{}, History: history})
	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey, downKey, enterKey)
	view := plainView(m)
	if !strings.Contains(view, "Tracks History") || !strings.Contains(view, "[ ] Auto-archive tracks") || strings.Contains(view, "Tracks with unsaved work") {
		t.Fatalf("Tracks History should show auto-archive off, without the unsaved-work field:\n%s", view)
	}

	m = settle(m, downKey, downKey, downKey, downKey, enterKey)
	if !history.saved.AutoArchive || !strings.Contains(plainView(m), "[x] Auto-archive tracks") {
		t.Fatalf("Enter on the checkbox: saved %+v", history.saved)
	}
	if view := plainView(m); !strings.Contains(view, "Tracks with unsaved work") || !strings.Contains(view, "Skip them") {
		t.Fatalf("the unsaved-work field should show, on Skip them:\n%s", view)
	}

	m = settle(m, downKey, enterKey, downKey, enterKey)
	if !history.saved.KeepUnsaved() || !strings.Contains(plainView(m), "Archive, remove nothing") {
		t.Fatalf("picking Archive, remove nothing: saved %+v", history.saved)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey)
	if history.saved.AutoArchive || strings.Contains(plainView(m), "Tracks with unsaved work") {
		t.Errorf("turning auto-archive off: saved %+v", history.saved)
	}
	if m = settle(m, downKey); m.settings.types.focus != typeAutoArchive {
		t.Errorf("focus went to %d past the hidden field", m.settings.types.focus)
	}
}
