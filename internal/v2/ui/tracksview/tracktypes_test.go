package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

type fakeTypes struct{ saved settings.Tracks }

func (f *fakeTypes) Load() (settings.Tracks, error) { return f.saved, nil }

func (f *fakeTypes) Save(t settings.Tracks) error {
	f.saved = t
	return nil
}

// openTypes opens Settings → Tracks with focus in it, on Claude Code
// with opus and one model of the user's.
func openTypes(t *testing.T) (Model, *fakeTypes) {
	t.Helper()
	engines := &fakeEngines{saved: settings.Engines{Claude: &settings.Engine{Model: "opus", Models: []string{"claude-opus-5-5"}}}}
	types := &fakeTypes{}
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), Engines: engines, TrackTypes: types})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey, downKey, enterKey), types
}

var (
	leftKey  = tea.KeyPressMsg{Code: tea.KeyLeft}
	rightKey = tea.KeyPressMsg{Code: tea.KeyRight}
)

func TestTrackTypeDefaults(t *testing.T) {
	m, types := openTypes(t)
	view := plainView(m)
	for _, want := range []string{"Track type default models", " Work ", " Doc ", "Claude Code", "Default (opus)"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}

	m = settle(m, rightKey, downKey, enterKey)
	if view := plainView(m); !strings.Contains(view, "Ask tracks: agent") || !strings.Contains(view, "not added") {
		t.Fatalf("the agent picker should list Cursor as not added:\n%s", view)
	}
	m = settle(m, downKey, enterKey)
	if d := types.saved.Get("ask"); d == nil || *d != (settings.TrackType{Engine: "cursor"}) {
		t.Fatalf("saved ask = %+v", d)
	}
	if view := plainView(m); !strings.Contains(view, "Cursor isn't added on the Engines tab, so Ask tracks can't start.") {
		t.Errorf("an agent that isn't added should say so:\n%s", view)
	}

	m = settle(m, leftKey, downKey, enterKey)
	view = plainView(m)
	for _, want := range []string{"Work tracks: model", "Claude Code's default: opus", "Sonnet (latest)", "claude-opus-5-5"} {
		if !strings.Contains(view, want) {
			t.Errorf("the model picker lacks %q:\n%s", want, view)
		}
	}
	for range 5 {
		m = settle(m, downKey)
	}
	m = settle(m, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "claude", Model: "claude-opus-5-5"}) {
		t.Fatalf("saved work = %+v", d)
	}
	if m.settings.notice.text != "Work tracks run on Claude Code, model claude-opus-5-5." {
		t.Errorf("notice %q", m.settings.notice.text)
	}

	m.engines.settings.Claude.Models = nil
	if view := plainView(m); !strings.Contains(view, "claude-opus-5-5") || !strings.Contains(view, notListed) {
		t.Errorf("a model the Engines tab dropped should say so:\n%s", view)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "cursor"}) {
		t.Errorf("another agent should start on its default model: %+v", d)
	}
}

func TestTrackTypeClicks(t *testing.T) {
	m, _ := openTypes(t)
	lines := strings.Split(plainView(m), "\n")
	click := func(label string, below int) {
		t.Helper()
		for y, line := range lines {
			if x := strings.Index(line, label); x >= 0 && strings.Contains(line, " Review ") {
				m = settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])) + 1, Y: y + below, Button: tea.MouseLeft})
				return
			}
		}
		t.Fatalf("%q not drawn", label)
	}
	click(" Plan ", 0)
	if m.selectedKind() != "plan" {
		t.Fatalf("clicking Plan selected %s", m.selectedKind())
	}
	lines = strings.Split(plainView(m), "\n")
	for y, line := range lines {
		if x := strings.Index(line, "Default (opus)"); x >= 0 {
			m = settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	if m.picker == nil || m.pickerFor != pickTypeModel || !strings.Contains(plainView(m), "Plan tracks: model") {
		t.Errorf("clicking the Model field should open its picker:\n%s", plainView(m))
	}
}
