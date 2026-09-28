package tracksview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

type fakeEngines struct {
	saved     settings.Engines
	found     map[string]agents.Found // a missing engine isn't installed
	models    []agents.Model
	modelsErr error
	mcp       []agents.MCPServer
	mcpErr    error
	checks    int
}

func (f *fakeEngines) Load() (settings.Engines, error) { return f.saved.Clone(), nil }

func (f *fakeEngines) Save(e settings.Engines) error {
	f.saved = e.Clone()
	return nil
}

func (f *fakeEngines) Check(_ context.Context, e agents.Engine) (agents.Found, error) {
	f.checks++
	found, ok := f.found[e.ID]
	if !ok {
		return agents.Found{}, fmt.Errorf("%s: %w", e.Program, agents.ErrNotFound)
	}
	return found, nil
}

func (f *fakeEngines) MCP(context.Context, string) ([]agents.MCPServer, error) {
	return f.mcp, f.mcpErr
}

func (f *fakeEngines) Models(context.Context, string) ([]agents.Model, error) {
	return f.models, f.modelsErr
}

// enginesTab opens the Engines tab on f, tall enough to show both boxes.
func openEngines(t *testing.T, f *fakeEngines) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Engines: f})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 70}, shiftTabKey, shiftTabKey)
}

func claudeFound() map[string]agents.Found {
	return map[string]agents.Found{"claude": {Path: "/bin/claude", Version: "2.1.281"}}
}

func TestAddingAnEngine(t *testing.T) {
	f := &fakeEngines{found: claudeFound()}
	m := openEngines(t, f)
	m = press(t, m, "Add Claude Code to Engines")
	view := plainView(m)
	for _, want := range []string{"active", "/bin/claude", "2.1.281", "Opus (latest)", "Disable auto mode", "Remove Claude Code"} {
		if !strings.Contains(view, want) {
			t.Errorf("an added engine should show %q", want)
		}
	}
	if f.saved.Claude == nil {
		t.Error("adding Claude Code should save it")
	}

	m = press(t, m, "Add Cursor to Engines")
	if view := plainView(m); !strings.Contains(view, "`agent` isn't on the PATH.") || !strings.Contains(view, "Check again") {
		t.Errorf("a missing CLI should say so and offer Check again:\n%s", view)
	}
	if f.saved.Cursor != nil {
		t.Error("a missing CLI shouldn't be added")
	}
	f.found["cursor"] = agents.Found{Path: "/bin/agent"}
	m = press(t, m, "Check again")
	if f.saved.Cursor == nil || !strings.Contains(plainView(m), "/bin/agent") {
		t.Error("Check again should add the engine once its CLI is there")
	}
}

func TestOpeningTheTabChecksAgain(t *testing.T) {
	on := false
	f := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{Auto: &on}}, found: claudeFound()}
	m := openEngines(t, f)
	if f.checks != 1 || !strings.Contains(plainView(m), "Enable auto mode") {
		t.Fatalf("opening the tab should read the settings and check Claude Code once, checked %d", f.checks)
	}
	delete(f.found, "claude")
	m = settle(m, tabKey, shiftTabKey)
	if f.checks != 2 || !strings.Contains(plainView(m), "not found") {
		t.Errorf("coming back should check again and show the CLI as not found, checked %d", f.checks)
	}
}

func TestClaudeModels(t *testing.T) {
	f := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{}}, found: claudeFound()}
	m := openEngines(t, f)
	x, y := cellOf(t, m, "[model id")
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = settle(typeText(m, "claude-opus-5-5"), enterKey)
	if got := f.saved.Claude.Models; len(got) != 1 || got[0] != "claude-opus-5-5" {
		t.Fatalf("saved models %v; want the typed id", got)
	}
	m = settle(typeText(m, "opus"), enterKey)
	if len(f.saved.Claude.Models) != 1 || !strings.Contains(plainView(m), "opus is already listed.") {
		t.Error("an alias already listed shouldn't be added again")
	}

	x, y = cellOf(t, m, "[ Default")
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.picker == nil || !strings.Contains(plainView(m), "claude-opus-5-5") {
		t.Fatal("the default model field should open the picker with the added models")
	}
	m = settle(typeText(m, "sonn"), enterKey)
	if f.saved.Claude.Model != "sonnet" || m.picker != nil {
		t.Errorf("filtering to sonnet and Enter saved %q", f.saved.Claude.Model)
	}

	f.saved.Claude.Model = "claude-opus-5-5"
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape}, tabKey, shiftTabKey)
	m = press(t, m, "Remove")
	if len(f.saved.Claude.Models) != 0 || f.saved.Claude.Model != "" {
		t.Errorf("removing the default's model saved %+v; want it gone and the default back", f.saved.Claude)
	}
}

func TestCursorListsItsModels(t *testing.T) {
	f := &fakeEngines{saved: settings.Engines{Cursor: &settings.Engine{}}, found: map[string]agents.Found{"cursor": {Path: "/bin/agent"}},
		modelsErr: errors.New("not logged in")}
	m := openEngines(t, f)
	x, y := cellOf(t, m, "[ Default")
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if !strings.Contains(plainView(m), "not logged in") {
		t.Fatal("the picker should show why the models couldn't be listed")
	}
	f.models, f.modelsErr = []agents.Model{{ID: "gpt-5.3-codex", Label: "Codex 5.3"}}, nil
	m = settle(settle(m, enterKey), downKey, enterKey)
	if f.saved.Cursor.Model != "gpt-5.3-codex" {
		t.Errorf("Enter should list the models again and choosing one save it, got %q", f.saved.Cursor.Model)
	}
}

func TestAutoModeAndRemoving(t *testing.T) {
	f := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{}}, found: claudeFound()}
	m := openEngines(t, f)
	m = press(t, m, "Disable auto mode")
	if f.saved.Claude.AutoMode() || !strings.Contains(plainView(m), "Enable auto mode") {
		t.Error("Disable auto mode should save auto mode off")
	}
	m = press(t, m, "Remove Claude Code")
	if f.saved.Claude == nil || !strings.Contains(plainView(m), "Remove Claude Code from Engines?") {
		t.Fatal("Remove should ask first")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(plainView(m), "from Engines?") {
		t.Error("Esc should cancel the removal")
	}
	m = press(t, m, "Remove Claude Code")
	x, y := cellOf(t, m, " Remove    Cancel ")
	m = settle(m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
	if f.saved.Claude != nil || !strings.Contains(plainView(m), "Add Claude Code to Engines") {
		t.Error("confirming should remove the engine and show its empty state")
	}
}

func TestEngineKeysMoveThroughControls(t *testing.T) {
	f := &fakeEngines{found: claudeFound()}
	m := openEngines(t, f)
	m = settle(m, enterKey)
	if !m.engines.editing || m.engines.focus.kind != ctlAdd {
		t.Fatal("Enter should move focus to the first control")
	}
	m = settle(m, enterKey)
	if f.saved.Claude == nil || m.engines.focus.kind != ctlModel {
		t.Fatalf("Enter on Add should add Claude Code and focus its model, focus %+v", m.engines.focus)
	}
	m = settle(m, tabKey, tabKey)
	if m.engines.focus.kind != ctlAddModel {
		t.Errorf("Tab twice from the model field should reach Add model, got %+v", m.engines.focus)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.engines.editing {
		t.Error("Esc should leave the controls")
	}
}

// cellOf is the first cell showing text.
func cellOf(t *testing.T, m Model, text string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(plainView(m), "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return len([]rune(line[:x])) + 1, y
		}
	}
	t.Fatalf("%q not drawn:\n%s", text, plainView(m))
	return 0, 0
}

func TestCheckingMCPServers(t *testing.T) {
	f := &fakeEngines{saved: settings.Engines{Cursor: &settings.Engine{}}, found: map[string]agents.Found{"cursor": {Path: "/bin/agent"}},
		mcpErr: errors.New("no network")}
	m := openEngines(t, f)
	m = press(t, m, "Check MCP servers")
	if !strings.Contains(plainView(m), "Couldn't check them: no network") {
		t.Fatal("a failed check should say why")
	}
	f.mcp, f.mcpErr = []agents.MCPServer{
		{Name: "linear", Status: "ready", State: agents.MCPReady},
		{Name: "context7", Status: "Error: Connection failed", State: agents.MCPFailed},
	}, nil
	m = press(t, m, "Check MCP servers")
	view := plainView(m)
	for _, want := range []string{"linear", "ready", "context7", "Error: Connection failed", "every project", "Cursor plugins"} {
		if !strings.Contains(view, want) {
			t.Errorf("the MCP list should show %q", want)
		}
	}
}
