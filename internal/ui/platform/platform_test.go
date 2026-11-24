package platform

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/theme"
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

func shown(t *testing.T, m Model, msgs ...tea.Msg) string {
	t.Helper()
	var tm tea.Model = m
	for _, msg := range append([]tea.Msg{tea.WindowSizeMsg{Width: 26, Height: 12}}, msgs...) {
		tm, _ = tm.Update(msg)
	}
	return escapes.ReplaceAllString(strings.Join(tm.(Model).lines(), "\n"), "")
}

func TestPlatformShowsTheTrains(t *testing.T) {
	m := New(Config{Theme: theme.Default()})
	view := shown(t, m, trainsMsg{trains: []rpc.Train{
		{Handle: "reviewer-527192", Agent: "Reviewer", StatusID: "done", OutcomeID: "blocked"},
		{Handle: "watch-000001", Agent: "Watcher", StatusID: "action_required"},
	}})
	for _, want := range []string{"╭─ Platform ", "reviewer-527192", " done   blocked ", "watch-000001", " action required "} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 12 {
		t.Errorf("%d lines, want the pane's 12", len(lines))
	}
	for i, l := range lines {
		if w := len([]rune(l)); w != 26 {
			t.Errorf("line %d is %d wide, want 26: %q", i, w, l)
		}
	}
}

func TestPlatformWithoutTrains(t *testing.T) {
	m := New(Config{Theme: theme.Default()})
	if view := shown(t, m, trainsMsg{}); !strings.Contains(view, "No trains.") {
		t.Errorf("no trains:\n%s", view)
	}
	if view := shown(t, m, trainsMsg{err: errors.New("the daemon is gone")}); !strings.Contains(view, "Couldn't read") {
		t.Errorf("an error:\n%s", view)
	}
}

func TestPlatformReadsAgainOnAChange(t *testing.T) {
	calls := 0
	m := New(Config{Theme: theme.Default(), Trains: func() ([]rpc.Train, error) { calls++; return nil, nil }})
	_, cmd := m.Update(changedMsg{})
	if cmd == nil {
		t.Fatal("a change read nothing")
	}
	for _, msg := range collect(cmd) {
		if _, ok := msg.(trainsMsg); ok && calls == 1 {
			return
		}
	}
	t.Errorf("a change read the trains %d times, want once", calls)
}

// collect runs cmd and the commands of a batch, without waiting on the
// ones that block.
func collect(cmd tea.Cmd) []tea.Msg {
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c != nil {
				out = append(out, c())
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestPlatformTakesANewThemeAndNoQuit(t *testing.T) {
	m := New(Config{Theme: theme.Default()})
	var light theme.Theme
	for _, b := range theme.BuiltIns() {
		if b.ID != theme.Default().ID {
			light = b
		}
	}
	tm, cmd := m.Update(themeMsg{theme: light})
	if tm.(Model).palette.Theme().ID != light.ID {
		t.Error("a theme read again wasn't taken")
	}
	_ = cmd
	if _, cmd := tm.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Error("ctrl+c quit the Platform, which the daemon would only open again")
		}
	}
}
