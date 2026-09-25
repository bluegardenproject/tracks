package tracksview

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

func update(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

var (
	tabKey      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTabKey = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
)

func TestSettingsShowsEveryToken(t *testing.T) {
	m := update(New("test", theme.Default(), nil), tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey)
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

func TestTabsWrapAround(t *testing.T) {
	m := New("test", theme.Default(), nil)
	for i := range len(tabs) {
		if m.tab != i {
			t.Fatalf("after %d Tab presses: tab %d", i, m.tab)
		}
		m = update(m, tabKey)
	}
	if m.tab != tabStation {
		t.Errorf("Tab on the last tab: tab %d, want Station", m.tab)
	}
	if m = update(m, shiftTabKey); m.tab != tabSettings {
		t.Errorf("Shift+Tab on Station: tab %d, want Settings", m.tab)
	}
}

func TestFrameFitsWindow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 40}, {80, 24}, {60, 15}, {200, 60}, {40, 10}} {
		for tab := range tabs {
			m := update(New("test", theme.Default(), nil), tea.WindowSizeMsg{Width: size.w, Height: size.h})
			m.tab = tab
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != size.h {
				t.Errorf("%dx%d, tab %s: %d lines", size.w, size.h, tabs[tab].title, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w != size.w {
					t.Errorf("%dx%d, tab %s: line %d is %d wide", size.w, size.h, tabs[tab].title, i, w)
					break
				}
			}
		}
	}
}

func TestShortWindowDropsBanner(t *testing.T) {
	m := update(New("test", theme.Default(), nil), tea.WindowSizeMsg{Width: 120, Height: 40})
	if !strings.Contains(m.View().Content, "v2 dev build") {
		t.Error("banner missing at 120x40")
	}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 12})
	if out := m.View().Content; strings.Contains(out, "v2 dev build") || !strings.Contains(out, "Station") {
		t.Error("at 120x12 the banner should give way to the tabs")
	}
}
