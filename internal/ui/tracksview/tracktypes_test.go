package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
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

func TestTrackTypeEffort(t *testing.T) {
	m, types := openTypes(t)
	if view := plainView(m); !strings.Contains(view, "Effort") {
		t.Fatalf("Claude Code's tracks should show an effort:\n%s", view)
	}
	m = settle(m, downKey, downKey, downKey, enterKey)
	view := plainView(m)
	for _, want := range []string{"Work tracks: effort", "Claude Code chooses", "xhigh"} {
		if !strings.Contains(view, want) {
			t.Errorf("the effort picker lacks %q:\n%s", want, view)
		}
	}
	m = settle(m, downKey, downKey, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "claude", Effort: "high"}) {
		t.Fatalf("saved work = %+v", d)
	}
	if m.settings.notice.text != "Work tracks run on Claude Code, effort high." {
		t.Errorf("notice %q", m.settings.notice.text)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey, downKey, downKey, downKey, downKey, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "claude", Model: "claude-opus-5-5", Effort: "high"}) {
		t.Errorf("a new model should keep the effort: %+v", d)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp}, enterKey, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "cursor"}) {
		t.Errorf("Cursor has no effort: %+v", d)
	}
	if view := plainView(m); strings.Contains(view, "Effort") {
		t.Errorf("Cursor's tracks shouldn't show an effort:\n%s", view)
	}
	m = settle(m, downKey, downKey)
	if m.settings.types.focus != typeAutoArchive {
		t.Errorf("down from Model should skip the hidden effort, focus %d", m.settings.types.focus)
	}
}

func TestTrackTypeEffortFocusWhenHidden(t *testing.T) {
	m, _ := openTypes(t)
	m = settle(m, downKey, downKey, downKey)
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "cursor"})
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.settings.types.focus != typeAgent {
		t.Errorf("Up from a hidden effort should reach Agent, focus %d", m.settings.types.focus)
	}
	m = settle(m, downKey, downKey)
	m.settings.types.focus = typeEffort
	m = settle(m, enterKey)
	if m.pickerFor == pickTypeEffort || m.settings.types.focus != typeModel {
		t.Errorf("Enter on a hidden effort should act on Model: picker %d, focus %d", m.pickerFor, m.settings.types.focus)
	}
}

func TestTrackTypeEffortSwitchingTypes(t *testing.T) {
	m, _ := openTypes(t)
	m.settings.types.tracks.Set("ask", &settings.TrackType{Engine: "claude", Effort: "turbo"})
	m = settle(m, downKey, downKey, downKey)
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "cursor"})
	m = settle(m, rightKey)
	if m.settings.types.focus != typeModel {
		t.Errorf("onto Ask from a hidden effort, focus %d; want Model, as drawn", m.settings.types.focus)
	}
	m = settle(m, downKey, enterKey)
	if view := plainView(m); !strings.Contains(view, "turbo") || !strings.Contains(view, "not a level Claude Code lists") || m.picker.Marked != len(m.picker.Items)-1 {
		t.Errorf("an unknown effort should be listed and marked, marked %d:\n%s", m.picker.Marked, view)
	}
}

func TestTrackTypeUnknownEffortWarns(t *testing.T) {
	m, _ := openTypes(t)
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "claude", Effort: "turbo"})
	view := plainView(m)
	if !strings.Contains(view, "turbo") || !strings.Contains(view, "not a level Claude Code lists") {
		t.Errorf("an unknown effort should say so beside its field:\n%s", view)
	}
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "claude", Effort: "high"})
	if view := plainView(m); strings.Contains(view, "not a level") {
		t.Errorf("a listed effort shouldn't warn:\n%s", view)
	}
}

func TestTrackTypeEffortClicks(t *testing.T) {
	m, _ := openTypes(t)
	effortAt := func() (x, y int, ok bool) {
		for y, line := range strings.Split(plainView(m), "\n") {
			if i := strings.Index(line, "Effort "); i >= 0 {
				if j := strings.Index(line[i:], "Default"); j >= 0 {
					return len([]rune(line[:i+j])), y, true
				}
			}
		}
		return 0, 0, false
	}
	x, y, ok := effortAt()
	if !ok {
		t.Fatalf("Effort not drawn:\n%s", plainView(m))
	}
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.picker == nil || m.pickerFor != pickTypeEffort || !strings.Contains(plainView(m), "Work tracks: effort") {
		t.Fatalf("clicking the Effort field should open its picker:\n%s", plainView(m))
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "cursor"})
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.picker != nil {
		t.Errorf("where a hidden Effort would be, a click opened %d:\n%s", m.pickerFor, plainView(m))
	}
}

func TestTrackTypePickersKeepTheirEngine(t *testing.T) {
	m, types := openTypes(t)
	m = settle(m, downKey, downKey, downKey, enterKey)
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "cursor", Model: "gpt-5"})
	m = settle(m, downKey, downKey, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "claude", Effort: "high"}) {
		t.Errorf("an effort picked for Claude Code after a reload moved Work to Cursor: %+v", d)
	}

	m.engines.models["cursor"] = []agents.Model{{ID: "gpt-5", Label: "GPT-5"}}
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "cursor"})
	m = settle(m, enterKey) // the hidden Effort's focus acts on Model
	if m.pickerFor != pickTypeModel || m.pickerEngine != "cursor" {
		t.Fatalf("the model picker didn't open on Cursor: %d, %q", m.pickerFor, m.pickerEngine)
	}
	m.settings.types.tracks.Set("work", &settings.TrackType{Engine: "claude", Effort: "high"})
	m = settle(m, downKey, enterKey)
	if d := types.saved.Get("work"); d == nil || *d != (settings.TrackType{Engine: "cursor", Model: "gpt-5"}) {
		t.Errorf("a model picked for Cursor after a reload moved Work to Claude Code took its effort: %+v", d)
	}
}
