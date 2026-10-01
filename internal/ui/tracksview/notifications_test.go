package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
)

type fakeNotify struct {
	saved settings.Notifications
	saves int
}

func (f *fakeNotify) Load() (settings.Notifications, error) { return f.saved, nil }

func (f *fakeNotify) Save(n settings.Notifications) error {
	f.saved, f.saves = n, f.saves+1
	return nil
}

func TestNotificationToggles(t *testing.T) {
	notify := &fakeNotify{saved: settings.Notifications{}.Set(settings.NotifyBell, false)}
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), Notifications: notify})
	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey)
	view := plainView(m)
	for _, want := range []string{"Notifications", "[x] macOS notifications", "[ ] Terminal bell", "[x] A track needs you", "[x] A track's PR is merged or closed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("General should show %q:\n%s", want, view)
		}
	}

	m = settle(m, enterKey, downKey, downKey, enterKey)
	if !notify.saved.On(settings.NotifyBell) || !strings.Contains(plainView(m), "[x] Terminal bell") || m.picker != nil {
		t.Fatalf("Enter on the bell's toggle: saved %+v, picker open %v", notify.saved, m.picker != nil)
	}
	if !strings.Contains(plainView(m), "Terminal bell on.") {
		t.Errorf("toggling should say what changed:\n%s", plainView(m))
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey); m.picker == nil {
		t.Error("Enter on the theme field should still open the theme picker")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	m = clickText(t, m, "[x] A track opens a PR")
	if notify.saved.On(settings.NotifyPROpened) || !strings.Contains(plainView(m), "[ ] A track opens a PR") {
		t.Fatalf("clicking the PR-opened toggle: saved %+v", notify.saved)
	}
	if !strings.Contains(plainView(m), "Not notifying when a track opens a PR.") {
		t.Errorf("toggling an event should say what changed:\n%s", plainView(m))
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyDown}); m.settings.generalFocus != len(notifyRows) {
		t.Errorf("focus went to %d past the last toggle", m.settings.generalFocus)
	}
	if notify.saves != 2 {
		t.Errorf("saved %d times, want once per toggle", notify.saves)
	}
}

// clickText clicks the first cell of text in the window.
func clickText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for y, line := range strings.Split(plainView(m), "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("%q not drawn", text)
	return m
}
