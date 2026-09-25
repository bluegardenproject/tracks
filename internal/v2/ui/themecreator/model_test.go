package themecreator

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// press sends keys and returns what the last one's command sends.
// Earlier commands, such as cursor blinks, aren't run.
func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) (Model, tea.Msg) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(k)
	}
	if cmd == nil || keys[len(keys)-1] != enter && keys[len(keys)-1] != esc {
		return m, nil
	}
	return m, cmd()
}

func typed(s string) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 0, len(s))
	for _, r := range s {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

func clearField() []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 8)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return keys
}

var (
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	right = tea.KeyPressMsg{Code: tea.KeyRight}
)

// toApply moves focus from the first field to the Apply button.
func toApply() []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, len(theme.All))
	for i := range keys {
		keys[i] = down
	}
	return keys
}

func TestApplySendsEditedTheme(t *testing.T) {
	edit := theme.Default().Value(theme.StateDanger).Dark
	m := New(theme.Default(), true)
	m, _ = press(t, m, clearField()...)
	m, _ = press(t, m, typed(edit)...)
	m, _ = press(t, m, tab)
	m, _ = press(t, m, clearField()...)
	m, _ = press(t, m, typed("#12")...) // invalid: keeps the old value

	m, _ = press(t, m, toApply()...)
	_, msg := press(t, m, enter)
	apply, ok := msg.(ApplyMsg)
	if !ok {
		t.Fatalf("Enter on Apply sent %T, want ApplyMsg", msg)
	}
	got := apply.Theme.Value(theme.TextDefault)
	if got.Dark != edit {
		t.Errorf("dark text.default = %q, want the edit", got.Dark)
	}
	if want := theme.Default().Value(theme.TextDefault).Light; got.Light != want {
		t.Errorf("light text.default = %q, want the old %q while the edit is invalid", got.Light, want)
	}
}

func TestCancelAndEscClose(t *testing.T) {
	m := New(theme.Default(), true)
	if _, msg := press(t, m, esc); msg != (CloseMsg{}) {
		t.Errorf("Esc sent %v, want CloseMsg", msg)
	}
	m, _ = press(t, m, toApply()...)
	m, _ = press(t, m, right)
	if _, msg := press(t, m, enter); msg != (CloseMsg{}) {
		t.Errorf("Enter on Cancel sent %v, want CloseMsg", msg)
	}
}

func TestListScrollsToFocus(t *testing.T) {
	m := New(theme.Default(), true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 90, Height: 16})
	keys := make([]tea.KeyPressMsg, len(theme.All)-1)
	for i := range keys {
		keys[i] = down
	}
	m, _ = press(t, m, keys...)
	last := string(theme.All[len(theme.All)-1])
	if out := m.View(); !strings.Contains(out, last) || strings.Count(out, "\n") != 15 {
		t.Errorf("focused on %s in a 16-line window: token not visible or wrong height:\n%s", last, out)
	}
}

func click(m Model, x, y int) (Model, tea.Cmd) {
	return m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestMouse(t *testing.T) {
	m := New(theme.Default(), true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	_, tokenLine := layout()
	bgBase := slices.Index(theme.All, theme.BgBase)

	m, _ = click(m, lightInputX+2, listTop+tokenLine[bgBase])
	if f := m.fields[m.focus]; f.token != theme.BgBase || f.dark {
		t.Fatalf("clicked bg.base's light value, focus is on %s (dark %v)", f.token, f.dark)
	}

	before := m.focus
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.offset == 0 || m.focus != before {
		t.Errorf("wheel: offset %d, focus %d; want the list scrolled and focus kept", m.offset, m.focus)
	}

	_, cmd := click(m, 2, m.buttonsY())
	first := cmd()
	msgs := []tea.Msg{first}
	if batch, ok := first.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				if msg := c(); msg != nil {
					msgs = append(msgs, msg)
				}
			}
		}
	}
	if !slices.ContainsFunc(msgs, func(msg tea.Msg) bool { _, ok := msg.(ApplyMsg); return ok }) {
		t.Errorf("clicking Apply sent %v, want an ApplyMsg", msgs)
	}
}

func TestLeftFieldKeepsNoCursor(t *testing.T) {
	m := New(theme.Default(), true)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 90, Height: 50})
	_, tokenLine := layout()
	i := slices.Index(theme.All, theme.BgBase)
	m, _ = click(m, lightInputX+3, listTop+tokenLine[i])
	m, _ = click(m, darkInputX+3, listTop+tokenLine[i+1])

	light := theme.Default().Value(theme.BgBase).Light
	if !strings.Contains(m.View(), light) {
		t.Errorf("the field that lost focus doesn't show %s in one piece", light)
	}
}
