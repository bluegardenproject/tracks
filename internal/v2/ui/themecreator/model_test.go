package themecreator

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

var (
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func key(s string) tea.KeyPressMsg { r := []rune(s)[0]; return tea.KeyPressMsg{Code: r, Text: s} }

// send delivers msgs and returns the messages the last one's command
// leads to, batches unpacked. Earlier commands, such as cursor blinks,
// aren't run.
func send(m Model, msgs ...tea.Msg) (Model, []tea.Msg) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		m, cmd = m.Update(msg)
	}
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		out = append(out, msg)
	}
	return m, out
}

func typed(s string) []tea.Msg {
	var keys []tea.Msg
	for _, r := range s {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

func clearField() []tea.Msg {
	keys := make([]tea.Msg, 8)
	for i := range keys {
		keys[i] = tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return keys
}

func find[T any](msgs []tea.Msg) (T, bool) {
	for _, msg := range msgs {
		if t, ok := msg.(T); ok {
			return t, true
		}
	}
	var zero T
	return zero, false
}

// creator is a focused creator editing t, 90 by 30 cells.
func creator(t theme.Theme) Model {
	m := New(t, theme.Default())
	m.SetSize(90, 30)
	m.Focus()
	return m
}

// mine is a user theme.
func mine() theme.Theme {
	t := theme.Default()
	t.ID, t.DisplayName, t.BuiltIn = "mine", "Mine", false
	return t
}

// toButtons moves focus from the first field to the first button.
func toButtons() []tea.Msg {
	keys := make([]tea.Msg, len(theme.All))
	for i := range keys {
		keys[i] = down
	}
	return keys
}

func TestSaveSendsTheEditedTheme(t *testing.T) {
	m := creator(mine())
	m, _ = send(m, clearField()...)
	m, _ = send(m, typed("#123456")...)
	m, _ = send(m, tab)
	m, _ = send(m, clearField()...)
	m, _ = send(m, typed("#12")...) // invalid: keeps the saved value
	if !m.Dirty() || slices.Index(m.buttons(), buttonSave) != 1 {
		t.Fatalf("after an edit: dirty %v, buttons %v; want Save after Load", m.Dirty(), m.buttons())
	}
	m, _ = send(m, toButtons()...)
	m, msgs := send(m, tea.KeyPressMsg{Code: tea.KeyRight}, enter)
	save, ok := find[SaveMsg](msgs)
	if !ok {
		t.Fatalf("Enter on Save sent %v, want SaveMsg", msgs)
	}
	if got := save.Theme.Value(theme.All[0]); got != "#123456" {
		t.Errorf("%s = %s, want the edit", theme.All[0], got)
	}
	if got, want := save.Theme.Value(theme.All[1]), mine().Value(theme.All[1]); got != want {
		t.Errorf("%s = %s, want the saved %s while the edit is invalid", theme.All[1], got, want)
	}

	m, _ = send(m, SavedMsg{Theme: save.Theme})
	if m.Dirty() || m.status != "Saved Mine." {
		t.Errorf("after saving: dirty %v, status %q", m.Dirty(), m.status)
	}
}

func TestBuiltInsSaveAsNew(t *testing.T) {
	m := creator(theme.Default())
	m, _ = send(m, clearField()...)
	m, _ = send(m, typed("#123456")...)
	if slices.Contains(m.buttons(), buttonSave) {
		t.Fatal("a built-in offers Save")
	}
	if m.Leave() || m.mode != modeLeave {
		t.Fatal("leaving with edits should ask")
	}
	m, _ = send(m, key("s"))
	if m.mode != modeName {
		t.Fatalf("Save on a built-in: mode %d, want naming a new theme", m.mode)
	}
	m, _ = send(m, typed("My Theme")...)
	m, msgs := send(m, enter)
	create, ok := find[CreateMsg](msgs)
	if !ok || create.Name != "My Theme" || create.Theme.Value(theme.All[0]) != "#123456" {
		t.Fatalf("Enter on the name sent %v, want CreateMsg with the edit", msgs)
	}

	m, _ = send(m, SavedMsg{Err: errors.New("a theme with this name exists")})
	if m.mode != modeName || !m.statusErr {
		t.Errorf("a failed create: mode %d, error %v; want to stay on the name", m.mode, m.statusErr)
	}
	created := create.Theme
	created.ID, created.DisplayName, created.BuiltIn = "my_theme", "My Theme", false
	_, msgs = send(m, SavedMsg{Theme: created})
	if _, ok := find[DoneMsg](msgs); !ok {
		t.Errorf("saving before leaving sent %v, want DoneMsg", msgs)
	}
}

func TestLeaving(t *testing.T) {
	m := creator(mine())
	if _, msgs := send(m, esc); len(msgs) != 1 || msgs[0] != (DoneMsg{}) {
		t.Errorf("Esc without edits sent %v, want DoneMsg", msgs)
	}
	m, _ = send(m, clearField()...)
	m, _ = send(m, typed("#123456")...)
	m, _ = send(m, esc)
	if m.mode != modeLeave || !strings.Contains(m.View(), "Unsaved changes.") {
		t.Fatal("Esc with edits should ask")
	}
	m, msgs := send(m, key("c"))
	if _, ok := find[StayMsg](msgs); !ok || m.mode != modeEdit || !m.Dirty() {
		t.Errorf("Cancel: %v, mode %d, dirty %v; want StayMsg and the edits kept", msgs, m.mode, m.Dirty())
	}
	m.Leave()
	m, msgs = send(m, key("d"))
	if _, ok := find[DoneMsg](msgs); !ok || m.Dirty() {
		t.Errorf("Discard: %v, dirty %v; want DoneMsg and the edits gone", msgs, m.Dirty())
	}
}

func TestLoad(t *testing.T) {
	m := creator(theme.Default())
	m, out := send(m, toButtons()...)
	m, out = send(m, enter)
	if !slices.ContainsFunc(out, func(msg tea.Msg) bool { _, ok := msg.(LoadMsg); return ok }) {
		t.Fatalf("Load sent %v; want LoadMsg, for the host to offer the themes", out)
	}
	light := theme.BuiltIns()[1]
	m.Load(light)
	if m.mode != modeEdit || m.Editing().ID != light.ID || m.Dirty() {
		t.Errorf("mode %d, editing %s; want %s in the editor", m.mode, m.Editing().ID, light.ID)
	}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestMouse(t *testing.T) {
	m := creator(mine())
	_, tokenLine := layout()
	bgBase := slices.Index(theme.All, theme.BgBase)

	m, _ = send(m, click(inputX+2, listTop+tokenLine[bgBase]))
	if m.fields[m.focus].token != theme.BgBase {
		t.Fatalf("clicked bg.base's value, focus is on %s", m.fields[m.focus].token)
	}

	m.SetSize(90, 16)
	before := m.focus
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.offset == 0 || m.focus != before {
		t.Errorf("wheel: offset %d, focus %d; want the list scrolled and focus kept", m.offset, m.focus)
	}

	x := m.buttonsX() + buttonWidth(buttonLoad) + widget.ButtonGap + 1
	m, _ = send(m, click(x, m.buttonsY()))
	if m.mode != modeName {
		t.Errorf("clicking Save as new: mode %d, want naming", m.mode)
	}
}

func TestLeftFieldKeepsNoCursor(t *testing.T) {
	m := creator(mine())
	m.SetSize(90, 60)
	_, tokenLine := layout()
	i := slices.Index(theme.All, theme.BgBase)
	m, _ = send(m, click(inputX+3, listTop+tokenLine[i]), click(inputX+3, listTop+tokenLine[i+1]))
	if v := mine().Value(theme.BgBase); !strings.Contains(m.View(), v) {
		t.Errorf("the field that lost focus doesn't show %s in one piece", v)
	}
}

func TestTokenNamesFitTheirColumn(t *testing.T) {
	for _, token := range theme.All {
		if len(token) >= nameWidth {
			t.Errorf("%s is %d long; nameWidth %d leaves no gap", token, len(token), nameWidth)
		}
	}
}

func TestExampleButtonsHover(t *testing.T) {
	m := creator(theme.Default())
	m.SetSize(100, 200)
	_, tokenLine := layout()
	y := listTop + tokenLine[slices.Index(theme.All, theme.ButtonBgDefault)] - 1
	_, starts := widget.ButtonRow(m.palette(), sampleButtons(-1)...)
	m, _ = send(m, tea.MouseMotionMsg{X: exampleX + starts[1] + 1, Y: y})
	if m.sampleHover != 1 {
		t.Fatalf("mouse on the accent example: hover %d, want 1", m.sampleHover)
	}
	m, _ = send(m, tea.MouseMotionMsg{X: 0, Y: y})
	if m.sampleHover != -1 {
		t.Errorf("mouse off the examples: hover %d, want none", m.sampleHover)
	}
}

func TestCtrlCCopiesTheValue(t *testing.T) {
	m := creator(theme.Default())
	_, out := send(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if len(out) != 1 {
		t.Fatalf("Ctrl+C on a value sent %v; want the clipboard write", out)
	}
	if m, _ = send(m, toButtons()...); m.focus < len(m.fields) {
		t.Fatal("focus should be on the buttons")
	}
	if _, out = send(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); len(out) != 0 {
		t.Errorf("Ctrl+C on a button sent %v; want nothing", out)
	}
}
