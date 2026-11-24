package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
)

type fakeAgents struct{ saved settings.Agents }

func (f *fakeAgents) Load() (settings.Agents, error) { return f.saved, nil }

func (f *fakeAgents) Save(a settings.Agents) error {
	f.saved = a
	return nil
}

// openAgents shows Settings → Trains (agents), on Claude Code with opus.
func openAgents(t *testing.T) (Model, *fakeAgents) {
	t.Helper()
	engines := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{Model: "opus"}}}
	a := &fakeAgents{}
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), Engines: engines, TrackTypes: &fakeTypes{}, Agents: a})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey, downKey, downKey), a
}

// clickOn clicks the first cell of text in the view.
func clickOn(t *testing.T, m Model, text string) Model {
	t.Helper()
	for y, line := range strings.Split(plainView(m), "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("%q not drawn:\n%s", text, plainView(m))
	return m
}

func TestReviewerSettings(t *testing.T) {
	m, a := openAgents(t)
	if m.settings.section != sectionAgents {
		t.Fatalf("section %d, want Trains (agents) after Tracks", m.settings.section)
	}
	view := plainView(m)
	for _, want := range []string{" Trains (agents) ", "─ Reviewer ─", sameAsTrack, "The track's", "It only reads"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}

	m = settle(m, enterKey, downKey)
	if m.settings.agents.focus != reviewerPrefix {
		t.Errorf("focus %d: the model can't take focus while the reviewer follows the track, so down goes to the prefix", m.settings.agents.focus)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.settings.agents.focus != reviewerAgent {
		t.Errorf("focus %d: up from the prefix skips the model", m.settings.agents.focus)
	}
	m = settle(m, enterKey)
	if view := plainView(m); !strings.Contains(view, "Reviewer: agent") || !strings.Contains(view, "not added") {
		t.Fatalf("Enter should open the agent picker, with Cursor not added:\n%s", view)
	}
	m = settle(m, downKey, downKey, enterKey)
	if d := a.saved.Reviewer; d == nil || *d != (settings.Agent{Engine: "cursor"}) {
		t.Fatalf("saved reviewer = %+v, want Cursor", d)
	}
	if m.settings.notice.text != "Reviews run on Cursor." {
		t.Errorf("notice %q", m.settings.notice.text)
	}
	if !strings.Contains(plainView(m), "Cursor isn't added on the Engines tab, so reviews can't run.") {
		t.Errorf("a reviewer that isn't added should say so:\n%s", plainView(m))
	}

	m = clickOn(t, m, "[ Cursor")
	if m.picker == nil || m.pickerFor != pickReviewerAgent {
		t.Fatalf("clicking the Agent field should open its picker:\n%s", plainView(m))
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey)
	if d := a.saved.Reviewer; d == nil || *d != (settings.Agent{Engine: "claude"}) {
		t.Fatalf("saved reviewer = %+v, want Claude Code", d)
	}
	m = clickOn(t, m, "Default (opus)")
	if m.picker == nil || m.pickerFor != pickReviewerModel || !strings.Contains(plainView(m), "Reviewer: model") {
		t.Fatalf("clicking the Model field should open its picker:\n%s", plainView(m))
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEsc})

	m = clickOn(t, m, "[ Claude Code")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey)
	if a.saved.Reviewer != nil {
		t.Errorf("picking %q saved %+v, want none", sameAsTrack, a.saved.Reviewer)
	}
}

func TestReviewerPrefix(t *testing.T) {
	m, a := openAgents(t)
	if !strings.Contains(plainView(m), "reviewer (its name)") {
		t.Fatalf("the prefix should show the reviewer's name by default:\n%s", plainView(m))
	}
	m = clickOn(t, m, "reviewer (its name)")
	if !m.settings.agents.editingPrefix {
		t.Fatal("clicking the prefix should edit it")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace},
		tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace},
		tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = settle(m, tea.PasteMsg{Content: "Bad Prefix"}, enterKey)
	if !m.settings.agents.editingPrefix || !strings.Contains(plainView(m), "isn't a prefix for trains") || a.saved.Reviewer != nil {
		t.Fatalf("a bad prefix should be refused, still editing, nothing saved (%+v):\n%s", a.saved.Reviewer, plainView(m))
	}
	m.settings.agents.prefix.SetValue("")
	m = settle(m, tea.PasteMsg{Content: "rev"}, enterKey)
	if m.settings.agents.editingPrefix || a.saved.Reviewer == nil || *a.saved.Reviewer != (settings.Agent{Prefix: "rev"}) {
		t.Fatalf("saved %+v, want the prefix rev alone", a.saved.Reviewer)
	}
	if m.settings.notice.text != "New reviewers take handles such as rev-527192." {
		t.Errorf("notice %q", m.settings.notice.text)
	}

	m = clickOn(t, m, "[ "+sameAsTrack)
	m = settle(m, downKey, enterKey)
	if d := a.saved.Reviewer; d == nil || *d != (settings.Agent{Engine: "claude", Prefix: "rev"}) {
		t.Fatalf("picking an agent lost the prefix: %+v", d)
	}
	m = clickOn(t, m, "[ Claude Code")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey)
	if d := a.saved.Reviewer; d == nil || *d != (settings.Agent{Prefix: "rev"}) {
		t.Fatalf("following the track again lost the prefix: %+v", d)
	}

	m = clickOn(t, m, " rev ")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.settings.agents.editingPrefix || m.settings.section != sectionAgents {
		t.Errorf("Esc should drop the edit and stay in the section (section %d)", m.settings.section)
	}
	m = clickOn(t, m, " rev ")
	m.settings.agents.prefix.SetValue("")
	m = settle(m, enterKey)
	if a.saved.Reviewer != nil {
		t.Errorf("clearing the prefix saved %+v, want nothing left", a.saved.Reviewer)
	}
}
