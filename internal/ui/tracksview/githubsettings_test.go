package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
)

type fakeGitHub struct {
	saved settings.GitHub
	saves int
}

func (f *fakeGitHub) Load() (settings.GitHub, error) { return f.saved, nil }

func (f *fakeGitHub) Save(g settings.GitHub) error {
	f.saved, f.saves = g, f.saves+1
	return nil
}

// openGitHub opens Settings → GitHub at width on f's settings.
func openGitHub(t *testing.T, f *fakeGitHub, width int) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), GitHub: f})
	m = settle(m, tea.WindowSizeMsg{Width: width, Height: 50}, shiftTabKey)
	m = settle(m, downKey, downKey, downKey)
	if m.settings.section != sectionGitHub {
		t.Fatalf("section %d, want GitHub", m.settings.section)
	}
	return m
}

// githubShown is the section's text, as drawn at the section's width.
func githubShown(m Model) []string {
	lines, _ := m.githubView(m.sectionWidth())
	for i, l := range lines {
		lines[i] = strings.TrimRight(escapes.ReplaceAllString(l, ""), " ")
	}
	return lines
}

// githubChoicesShown is the choices drawn under each row's label,
// their lines joined.
func githubChoicesShown(t *testing.T, m Model) map[string]string {
	t.Helper()
	lines := githubShown(m)
	shown := map[string]string{}
	for i, line := range lines {
		for _, r := range githubRows {
			if line != r.label {
				continue
			}
			var got []string
			for _, next := range lines[i+1:] {
				if !strings.HasPrefix(next, "  (") {
					break
				}
				got = append(got, strings.TrimSpace(next))
			}
			shown[r.label] = strings.Join(got, "  ")
		}
	}
	return shown
}

func TestGitHubSectionShowsTheDefaults(t *testing.T) {
	m := openGitHub(t, &fakeGitHub{saved: settings.GitHub{CIFailed: "sometimes"}}, 140)
	if !strings.Contains(plainView(m), "GitHub") {
		t.Error("Settings should list GitHub")
	}
	view := strings.Join(githubShown(m), " ")
	for _, want := range []string{"on a side track's PR", "main track always hears of it", "Ask me: the main track asks you.", "Merged and closed PRs always go to the main track."} {
		if !strings.Contains(view, want) {
			t.Errorf("GitHub should show %q:\n%s", want, view)
		}
	}
	shown := githubChoicesShown(t, m)
	for label, want := range map[string]string{
		"Conflicts with its base": "(•) Handle automatically  ( ) Main track decides  ( ) Ask me",
		"CI failed":               "(•) Handle automatically  ( ) Main track decides  ( ) Ask me",
		"New review threads":      "( ) Handle automatically  (•) Main track decides  ( ) Ask me",
	} {
		if !strings.Contains(shown[label], want) {
			t.Errorf("%s shows %q, want %q", label, shown[label], want)
		}
	}
}

func TestGitHubKeysSave(t *testing.T) {
	f := &fakeGitHub{saved: settings.GitHub{ReviewThreads: "later"}}
	m := openGitHub(t, f, 140)
	m = settle(m, enterKey, downKey, tea.KeyPressMsg{Code: tea.KeyRight})
	if f.saved.CIFailed != settings.PolicyMain || f.saved.ReviewThreads != "later" {
		t.Fatalf("right on CI failed saved %+v; want main, and review threads' unknown value kept", f.saved)
	}
	if !strings.Contains(plainView(m), "CI failed: the main track decides.") {
		t.Errorf("changing a policy should say so:\n%s", plainView(m))
	}
	m = settle(m, enterKey)
	if f.saved.CIFailed != settings.PolicyAsk || !strings.Contains(plainView(m), "CI failed: Tracks asks you.") {
		t.Errorf("enter should cycle on to ask: saved %+v", f.saved)
	}
	m = settle(m, enterKey)
	if f.saved.CIFailed != settings.PolicyAuto {
		t.Errorf("enter should cycle back to auto: saved %+v", f.saved)
	}
	saves := f.saves
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyLeft}, downKey, downKey, downKey)
	if f.saves != saves || m.settings.github.focus != len(githubRows)-1 {
		t.Errorf("left on the first choice saved (%d saves, want %d), or focus %d went past the last row", f.saves, saves, m.settings.github.focus)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if f.saved.ReviewThreads != settings.PolicyAuto || f.saved.CIFailed != settings.PolicyAuto {
		t.Errorf("left on review threads saved %+v", f.saved)
	}
	if !strings.Contains(githubChoicesShown(t, m)["New review threads"], "(•) Handle automatically") {
		t.Errorf("the view should show the new policy:\n%s", plainView(m))
	}
}

func TestGitHubClickSaves(t *testing.T) {
	f := &fakeGitHub{}
	m := openGitHub(t, f, 140)
	m = clickText(t, m, "( ) Ask me")
	if f.saved.Conflicts != settings.PolicyAsk || f.saved.CIFailed != "" || f.saved.ReviewThreads != "" {
		t.Fatalf("clicking conflicts' Ask me saved %+v; want only conflicts written", f.saved)
	}
	if !m.settings.editing || !strings.Contains(plainView(m), "Conflicts with its base: Tracks asks you.") {
		t.Errorf("a click should focus the section and say what changed:\n%s", plainView(m))
	}
	m = clickText(t, m, "(•) Ask me")
	if f.saves != 1 {
		t.Errorf("clicking the chosen policy saved again: %d saves", f.saves)
	}
}

func TestGitHubNarrow(t *testing.T) {
	f := &fakeGitHub{}
	m := openGitHub(t, f, 34)
	m = settle(m, enterKey)
	for i, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d wide in a %d window: %q", i, w, m.width, escapes.ReplaceAllString(line, ""))
		}
	}
	shown := githubChoicesShown(t, m)
	for _, r := range githubRows {
		if strings.Count(shown[r.label], "(") != len(githubChoices) {
			t.Errorf("%s shows %q; want all %d choices, wrapped", r.label, shown[r.label], len(githubChoices))
		}
	}
	m = clickText(t, m, "( ) Main track")
	if f.saved.Conflicts != settings.PolicyMain {
		t.Errorf("clicking a wrapped choice saved %+v", f.saved)
	}
}
