package addtrack

import (
	"os"
	"path/filepath"
	"strings"
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

func TestCreateSaysItIsNotBuiltAndStays(t *testing.T) {
	m := form("tracks")
	m = m.setFocus(ctlRepos)
	m, _ = send(m, space)
	m = m.setFocus(ctlPrompt)
	m, _ = send(m, typed("Fix the rate bug")...)
	m = m.setFocus(ctlCreate)
	m, cmd := send(m, enter)
	if len(m.errs) != 0 || m.notice != notBuilt || quits(cmd) {
		t.Errorf("problems %v, notice %q, quit %v", m.errs, m.notice, quits(cmd))
	}
	if !strings.Contains(m.hints(), notBuilt) {
		t.Errorf("hint row %q doesn't show the notice", m.hints())
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

func TestDocumentProblem(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	deck := filepath.Join(dir, "deck.pptx")
	for _, f := range []string{doc, deck} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]string{
		doc:                        "",
		dir:                        "",
		deck:                       "PowerPoint files can't be read directly. Export it to PDF and pick that.",
		"":                         "Enter the document's path.",
		filepath.Join(dir, "gone"): "There's no file or folder at " + filepath.Join(dir, "gone") + ".",
	} {
		if got := documentProblem(path); got != want {
			t.Errorf("documentProblem(%q) = %q, want %q", path, got, want)
		}
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
	m = click(m, ctlRepos, 1)
	if !m.picked["ledger-live"] || m.focus != ctlRepos {
		t.Errorf("clicking a repo: picked %v, focus %d", m.picked, m.focus)
	}
	if m = click(m, ctlType, int(Doc)); m.kind != Doc {
		t.Errorf("clicking Doc review left kind %d", m.kind)
	}
	if m = click(m, ctlCandor, 0); m.picker == nil {
		t.Error("clicking Candor didn't open its picker")
	}
}
