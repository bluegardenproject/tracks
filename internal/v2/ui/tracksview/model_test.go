package tracksview

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

func update(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestRendersEveryToken(t *testing.T) {
	m := update(New("test", theme.Default(), nil), tea.WindowSizeMsg{Width: 120, Height: 40})
	out := m.View().Content
	for _, token := range theme.All {
		if !strings.Contains(out, string(token)) {
			t.Errorf("token %s missing from the view", token)
		}
	}
}

func TestFollowsTerminalBackground(t *testing.T) {
	m := update(New("test", theme.Default(), nil), tea.BackgroundColorMsg{Color: color.White})
	if m.palette.Dark() {
		t.Error("white background: palette still dark")
	}
	m = update(m, tea.BackgroundColorMsg{Color: color.Black})
	if !m.palette.Dark() {
		t.Error("black background: palette not dark")
	}
}

func TestKeysDontQuit(t *testing.T) {
	m := New("test", theme.Default(), nil)
	for _, key := range []tea.KeyPressMsg{{Code: 'q'}, {Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEscape}} {
		if _, cmd := m.Update(key); cmd != nil {
			t.Errorf("key %s returned a command", key)
		}
	}
}

func TestTinyWindow(t *testing.T) {
	m := update(New("test", theme.Default(), nil), tea.WindowSizeMsg{Width: 10, Height: 3})
	_ = m.View()
}
