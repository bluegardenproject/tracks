package addtrack

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

var (
	tab    = tea.KeyPressMsg{Code: tea.KeyTab}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc    = tea.KeyPressMsg{Code: tea.KeyEscape}
	space  = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	right  = tea.KeyPressMsg{Code: tea.KeyRight}
	left   = tea.KeyPressMsg{Code: tea.KeyLeft}
	resize = tea.WindowSizeMsg{Width: 96, Height: 32}
)

func form(repos ...string) Model {
	m, _ := send(New(Config{Theme: theme.Default(), Repos: repos}), resize)
	return m
}

// send delivers msgs and returns the last one's command.
func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func typed(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// create focuses Create and presses it.
func create(m Model) Model {
	m = m.setFocus(ctlCreate)
	m, _ = send(m, enter)
	return m
}

func TestTypeSwapsAnUntouchedPromptAndKeepsValues(t *testing.T) {
	m := form("tracks")
	m, _ = send(m, append([]tea.Msg{tab, tab}, typed("rates")...)...)
	m = m.setFocus(ctlType)
	m, _ = send(m, right, right)
	if m.kind != Plan || m.prompt.Value() != kinds[Plan].prompt {
		t.Fatalf("kind %d with prompt %q, want Plan's template", m.kind, m.prompt.Value())
	}
	if m.name.Value() != "rates" {
		t.Errorf("name %q didn't carry over", m.name.Value())
	}
	m = m.setFocus(ctlPrompt)
	m, _ = send(m, typed("Cache the rates.")...)
	edited := m.prompt.Value()
	m = m.setFocus(ctlType)
	m, _ = send(m, right)
	if m.kind != Review || m.prompt.Value() != edited {
		t.Errorf("an edited prompt changed on switching type: %q", m.prompt.Value())
	}
}

func TestCreateChecksTheFieldsAsV1(t *testing.T) {
	m := create(form("tracks"))
	if m.errs[ctlRepos] == "" || m.errs[ctlPrompt] == "" || m.focus != ctlRepos {
		t.Errorf("Work without repo or prompt: problems %v, focus %d", m.errs, m.focus)
	}

	m = form()
	m = m.setFocus(ctlType)
	m, _ = send(m, right)
	m = create(m)
	if m.errs[ctlRepos] != "" || m.errs[ctlPrompt] != "Enter a question." {
		t.Errorf("Ask: problems %v, want only the question", m.errs)
	}

	m = form("tracks")
	m = m.setFocus(ctlType)
	m, _ = send(m, right, right, right)
	m = create(m)
	if m.errs[ctlTarget] == "" || m.errs[ctlRepo] != "" || m.errs[ctlPrompt] != "" {
		t.Errorf("Review: problems %v, want the PR or branch only", m.errs)
	}

	m = form()
	m = m.setFocus(ctlType)
	m, _ = send(m, right, right, right)
	if m = create(m); m.errs[ctlRepo] != noRepos {
		t.Errorf("Review without repos: problems %v", m.errs)
	}
}

func TestEscAsksBeforeDiscarding(t *testing.T) {
	if _, cmd := send(form("tracks"), esc); !quits(cmd) {
		t.Fatal("Esc on an untouched form didn't close it")
	}
	m := form("tracks")
	m = m.setFocus(ctlName)
	m, _ = send(m, typed("x")...)
	m, cmd := send(m, esc)
	if m.discard == nil || quits(cmd) {
		t.Fatal("Esc with a name entered didn't ask")
	}
	if m, cmd = send(m, enter); m.discard != nil || quits(cmd) {
		t.Fatal("Enter on Keep editing didn't go back to the form")
	}
	m, _ = send(m, esc, left)
	if _, cmd = send(m, enter); !quits(cmd) {
		t.Error("Discard didn't close the form")
	}
}

func TestClicks(t *testing.T) {
	m := form("tracks", "ledger-live")
	click := func(m Model, c control, index int) Model {
		t.Helper()
		_, hits := m.body(m.width - 4)
		for _, h := range hits {
			if h.ctl == c && h.index == index {
				m, _ = send(m, tea.MouseClickMsg{X: 2 + h.x, Y: 1 + h.y - m.offset, Button: tea.MouseLeft})
				return m
			}
		}
		t.Fatalf("no control %d item %d on the form", c, index)
		return m
	}
	if m = click(m, ctlRepos, 0); m.picker == nil || m.focus != ctlRepos {
		t.Fatal("clicking Select repos didn't open the picker")
	}
	x, y, _, _ := m.pickerBox()
	m, _ = send(m, tea.MouseClickMsg{X: x + 2, Y: y + 2, Button: tea.MouseLeft})
	if m.picker == nil {
		t.Fatal("ticking a repo closed the picker")
	}
	m, _ = send(m, tea.MouseClickMsg{X: x + 2, Y: y + 4, Button: tea.MouseLeft})
	if m.picker != nil || !m.picked["ledger-live"] || m.picked["tracks"] {
		t.Fatalf("ticking ledger-live and OK: picker open %v, picked %v", m.picker != nil, m.picked)
	}
	if m = click(m, ctlRepos, 1); len(m.pickedRepos()) != 0 {
		t.Errorf("clicking the ✕ left %v", m.pickedRepos())
	}
	if m = click(m, ctlType, int(Doc)); m.kind != Doc {
		t.Errorf("clicking Doc review left kind %d", m.kind)
	}
	if m = click(m.setFocus(ctlCandor).follow(), ctlCandor, 0); m.picker == nil {
		t.Error("clicking Candor didn't open its picker")
	}
}
