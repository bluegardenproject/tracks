package tracksview

import (
	"regexp"
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

func TestKeysDontQuit(t *testing.T) {
	m := New(Config{Version: "test", Theme: theme.Default()})
	for _, key := range []tea.KeyPressMsg{{Code: 'q'}, {Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEscape}} {
		if _, cmd := m.Update(key); cmd != nil {
			t.Errorf("key %s returned a command", key)
		}
	}
}

func TestTinyWindow(t *testing.T) {
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 10, Height: 3})
	_ = m.View()
}

func TestTabsWrapAround(t *testing.T) {
	m := New(Config{Version: "test", Theme: theme.Default()})
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
			m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: size.w, Height: size.h})
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
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: 40})
	if !strings.Contains(m.View().Content, "v2 dev build") {
		t.Error("banner missing at 120x40")
	}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 12})
	if out := m.View().Content; strings.Contains(out, "v2 dev build") || !strings.Contains(out, "Station") {
		t.Error("at 120x12 the banner should give way to the tabs")
	}
}

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// clickLabel clicks where title is drawn in the tab row.
func clickLabel(t *testing.T, m Model, title string) Model {
	t.Helper()
	for y, line := range strings.Split(escapes.ReplaceAllString(m.View().Content, ""), "\n") {
		if x := strings.Index(line, "│  "+title+"  │"); x >= 0 {
			return update(m, tea.MouseClickMsg{X: len([]rune(line[:x])) + 3, Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("tab %s not drawn", title)
	return m
}

func TestClickSelectsTab(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 40}, {120, 12}} {
		m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: size.w, Height: size.h})
		for i, tb := range tabs {
			if m = clickLabel(t, m, tb.title); m.tab != i {
				t.Errorf("%dx%d: clicked %s, tab is %s", size.w, size.h, tb.title, tabs[m.tab].title)
			}
		}
		if m = update(m, tea.MouseClickMsg{X: 1, Y: m.tabsTop() + 1, Button: tea.MouseLeft}); m.tab != tabSettings {
			t.Errorf("%dx%d: a click left of the tabs changed the tab", size.w, size.h)
		}
	}
}
