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
	if m.settings.agents.focus != reviewerAgent {
		t.Error("the model can't take focus while the reviewer follows the track")
	}
	m = settle(m, enterKey)
	if view := plainView(m); !strings.Contains(view, "Reviewer: agent") || !strings.Contains(view, "not added") {
		t.Fatalf("Enter should open the agent picker, with Cursor not added:\n%s", view)
	}
	m = settle(m, downKey, downKey, enterKey)
	if d := a.saved.Reviewer; d == nil || *d != (settings.TrackType{Engine: "cursor"}) {
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
	if d := a.saved.Reviewer; d == nil || *d != (settings.TrackType{Engine: "claude"}) {
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
