package addtrack

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

var cursorAgent = Engine{ID: "cursor", Name: "Cursor", Lists: true}

// withCursor is a form with Claude Code and Cursor added; Cursor's
// models come from list, and Init hasn't been run.
func withCursor(list ModelsFunc) Model {
	m, _ := send(New(Config{
		Theme: theme.Default(), Repos: []string{"tracks"},
		RunsOn:  map[track.Kind]RunsOn{track.Work: {Engine: "claude", Model: "opus"}, track.Ask: {Engine: "cursor", Model: "gpt-5"}},
		Engines: []Engine{claudeCode, cursorAgent}, Models: list,
	}), resize)
	return m
}

func labels(p *widget.Picker) []string {
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Label+" "+it.Detail)
	}
	return out
}

// choose opens c's picker and picks the item labelled label.
func choose(t *testing.T, m Model, c control, label string) Model {
	t.Helper()
	m = m.setFocus(c)
	m, _ = send(m, enter)
	if m.picker == nil {
		t.Fatalf("no picker for %d", c)
	}
	for i, it := range m.picker.Items {
		if it.Label == label {
			m, _ = m.pickerDone(pick(m.picker, i))
			return m
		}
	}
	t.Fatalf("%q isn't offered: %q", label, labels(m.picker))
	return m
}

func pick(p *widget.Picker, i int) widget.PickerResult {
	p.Cursor = i
	return widget.PickerChosen
}

func TestPickingTheModel(t *testing.T) {
	m := ready(t, nil).setFocus(ctlModel)
	m, _ = send(m, enter)
	want := []string{"Default Claude Code's default: sonnet"}
	for _, model := range agents.Claude.Models {
		want = append(want, model.ID+" "+model.Label)
	}
	want = append(want, "claude-opus-4-8 added")
	if got := labels(m.picker); len(got) != len(want) || got[0] != want[0] || got[len(got)-1] != want[len(want)-1] {
		t.Errorf("models %q, want %q", got, want)
	}
	if it := m.picker.Items[m.picker.Marked]; it.Label != "opus" {
		t.Errorf("the type's model isn't marked: %q", it.Label)
	}
	m, _ = m.pickerDone(pick(m.picker, len(want)-1))
	if m.model != "claude-opus-4-8" || m.request().Model != "claude-opus-4-8" {
		t.Errorf("picked %q, requests %q", m.model, m.request().Model)
	}
	if m = m.setKind(Ask); m.engine != "claude" || m.model != "claude-opus-4-8" {
		t.Errorf("another type dropped the pick: %s %q", m.engine, m.model)
	}
	if !m.dirty() {
		t.Error("a picked model isn't something Esc would lose")
	}

	m = choose(t, m, ctlModel, "Default")
	if m.model != "" || m.modelText() != "Default (sonnet)" {
		t.Errorf("Default picked: %q shown as %q", m.model, m.modelText())
	}
}

func TestPickingTheAgent(t *testing.T) {
	m := withCursor(nil)
	m = choose(t, m, ctlEngine, "Cursor")
	if m.engine != "cursor" || m.model != "" {
		t.Errorf("another agent should start on Default: %s %q", m.engine, m.model)
	}
	m = choose(t, m, ctlEngine, "Claude Code")
	if m.engine != "claude" || m.model != "opus" {
		t.Errorf("back on the type's agent should be the type's model: %s %q", m.engine, m.model)
	}
	if r := m.request(); r.Engine != "claude" || r.Model != "opus" {
		t.Errorf("request runs on %s %q", r.Engine, r.Model)
	}
}

func TestAnAgentThatIsNotAddedCantCreate(t *testing.T) {
	created := false
	m := ready(t, func(context.Context, tracks.Request, func(string)) (Created, error) {
		created = true
		return Created{}, nil
	}).setKind(Ask)
	m = m.setFocus(ctlCreate)
	m, cmd := send(m, enter)
	if cmd != nil || m.creating != nil || created || m.focus != ctlEngine || m.notice != m.engineProblem() {
		t.Errorf("created %v, focus %d, notice %q", m.creating != nil, m.focus, m.notice)
	}
	m = m.setFocus(ctlEngine)
	m, _ = send(m, enter)
	if got := labels(m.picker); len(got) != 2 || got[1] != "Cursor not added" {
		t.Errorf("agents %q", got)
	}
}

func TestCursorsModelsLoadInTheBackground(t *testing.T) {
	calls := 0
	fail := true
	m := withCursor(func(_ context.Context, id string) ([]agents.Model, error) {
		calls++
		if fail {
			return nil, errors.New("not logged in.")
		}
		return []agents.Model{{ID: "gpt-5", Label: "GPT-5"}, {ID: "sonnet-4.5", Label: "Sonnet 4.5"}}, nil
	})
	list := m.Init()
	if list == nil {
		t.Fatal("Init doesn't ask Cursor for its models")
	}

	m = m.setKind(Ask).setFocus(ctlModel)
	m, _ = send(m, enter)
	if m.picker == nil || m.picker.Message != "Loading models…" {
		t.Fatalf("the picker should wait for the list: %+v", m.picker)
	}
	m, _ = send(m, list())
	if !m.picker.Problem || m.picker.Message != "Couldn't list the models: not logged in. Enter tries again." {
		t.Fatalf("a failed list: %q", m.picker.Message)
	}

	fail = false
	m, retry := send(m, enter)
	if retry == nil || m.picker.Message != "Loading models…" {
		t.Fatalf("Enter should list again: %q", m.picker.Message)
	}
	m, _ = send(m, retry())
	if got := labels(m.picker); len(got) != 3 || got[1] != "gpt-5 GPT-5" || m.picker.Items[m.picker.Marked].Label != "gpt-5" {
		t.Errorf("models %q, marked %d", got, m.picker.Marked)
	}
	m, _ = send(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.picker != nil || m.model != "sonnet-4.5" || calls != 2 {
		t.Errorf("picked %q after %d lists", m.model, calls)
	}
}

func TestTheRunsOnRowIsOneStop(t *testing.T) {
	m := ready(t, nil).setFocus(ctlEngine)
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want control
	}{
		{tea.KeyPressMsg{Code: tea.KeyRight}, ctlModel},
		{tea.KeyPressMsg{Code: tea.KeyUp}, ctlPrompt},
		{tea.KeyPressMsg{Code: tea.KeyTab}, ctlEngine},
		{tea.KeyPressMsg{Code: tea.KeyTab}, ctlModel},
		{tea.KeyPressMsg{Code: tea.KeyLeft}, ctlEngine},
		{tea.KeyPressMsg{Code: tea.KeyDown}, ctlCreate},
	} {
		if m, _ = send(m, step.key); m.focus != step.want {
			t.Fatalf("%s moved to %d, want %d", step.key, m.focus, step.want)
		}
	}
}
