package platform

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
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
	view := shown(t, m, trainsMsg{trains: []Entry{
		{Handle: "reviewer-527192", Status: track.Done, Outcome: train.Blocked},
		{Handle: "watch-000001", Status: track.ActionRequired},
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

func TestALongHandleEndsInAnEllipsis(t *testing.T) {
	m := New(Config{Theme: theme.Default()})
	view := shown(t, m, trainsMsg{trains: []Entry{{Handle: "a-very-long-prefix-for-trains-123456", Status: track.Active}}})
	if !strings.Contains(view, "a-very-long-prefix-fo…") {
		t.Errorf("a long handle:\n%s", view)
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
	m := New(Config{Theme: theme.Default(), Trains: func() ([]Entry, error) { calls++; return nil, nil }})
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

// collect runs cmd and the commands of a batch. A command that blocks,
// such as waiting for a change with a Watch, blocks it too: the tests
// give the Model none.
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
	m := New(Config{Theme: theme.Default(), LoadTheme: func() (theme.Theme, error) { return theme.Default(), nil }})
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
	if cmd == nil {
		t.Error("the theme isn't read again after a read")
	}
	if _, cmd := tm.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Error("ctrl+c quit the Platform, which the daemon would only open again")
		}
	}
}

func TestCollectMessages(t *testing.T) {
	collected := 0
	m := New(Config{Theme: theme.Default(), Collect: func() error { collected++; return errors.New("nothing waits") }})
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 26, Height: 12})
	tm, _ = tm.Update(trainsMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done, Held: true}}})
	view := escapes.ReplaceAllString(strings.Join(tm.(Model).lines(), "\n"), "")
	if !strings.Contains(view, "Collect messages") {
		t.Fatalf("a held train has no Collect messages:\n%s", view)
	}
	row := -1
	for i, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "Collect messages") {
			row = i
		}
	}
	tm, cmd := tm.Update(tea.MouseClickMsg{X: 4, Y: row, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on Collect messages did nothing")
	}
	tm, _ = tm.Update(cmd())
	if collected != 1 || !strings.Contains(escapes.ReplaceAllString(strings.Join(tm.(Model).lines(), "\n"), ""), "nothing waits") {
		t.Errorf("collected %d times; the notice should say why it couldn't", collected)
	}
	if _, cmd := tm.Update(tea.MouseClickMsg{X: 4, Y: 1, Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click on the train's name collected")
	}
}

func TestCollectClicksAreSafe(t *testing.T) {
	calls := 0
	m := New(Config{Theme: theme.Default(), Collect: func() error { calls++; return nil }})
	var tm tea.Model = m
	tm, _ = tm.Update(trainsMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done, Held: true}}})
	if _, cmd := tm.Update(tea.MouseClickMsg{X: 4, Y: 3, Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click before the pane's size is known collected")
	}
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 4, Height: 12})
	if _, cmd := tm.Update(tea.MouseClickMsg{X: 1, Y: 3, Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click in a pane too small to draw collected")
	}
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 26, Height: 12})
	for _, x := range []int{2, 24} {
		if _, off := tm.Update(tea.MouseClickMsg{X: x, Y: 3, Button: tea.MouseLeft}); off != nil {
			t.Errorf("a click at x=%d, beside the button, collected", x)
		}
	}
	tm, first := tm.Update(tea.MouseClickMsg{X: 4, Y: 3, Button: tea.MouseLeft})
	if first == nil {
		t.Fatal("a click on the button did nothing")
	}
	if _, again := tm.Update(tea.MouseClickMsg{X: 4, Y: 3, Button: tea.MouseLeft}); again != nil {
		t.Error("a second click while the first is on its way collected again")
	}
	tm, _ = tm.Update(collectedMsg{err: errors.New("answer it first")})
	tm, _ = tm.Update(trainsMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done}}})
	if strings.Contains(escapes.ReplaceAllString(strings.Join(tm.(Model).lines(), "\n"), ""), "answer it first") {
		t.Error("the notice of a button that's gone stayed")
	}
}

// A click on a train's handle or badges opens it; the border, the blank
// rows, Collect messages and below the list don't. A held train's
// button pushes the trains after it down a row.
func TestAClickOpensTheTrain(t *testing.T) {
	var opened []string
	m := New(Config{Theme: theme.Default(), Open: func(h string) { opened = append(opened, h) }})
	var tm tea.Model = m
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 26, Height: 12}, trainsMsg{trains: []Entry{
		{Handle: "reviewer-527192", Status: track.Active, Held: true}, {Handle: "watch-000001", Status: track.Done}}}} {
		tm, _ = tm.Update(msg)
	}
	click := func(y int) {
		t.Helper()
		var cmd tea.Cmd
		tm, cmd = tm.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		if cmd != nil {
			if msg := cmd(); msg != nil {
				if _, ok := msg.(openedMsg); ok {
					tm, _ = tm.Update(msg)
				}
			}
		}
	}
	for _, y := range []int{0, 1, 2, 4, 5, 6, 7, 8, 11} {
		click(y)
	}
	for _, x := range []int{0, 25} {
		if _, cmd := tm.Update(tea.MouseClickMsg{X: x, Y: 1, Button: tea.MouseLeft}); cmd != nil {
			t.Errorf("a click on the frame's border, column %d, opened a train", x)
		}
	}
	want := "reviewer-527192 reviewer-527192 watch-000001 watch-000001"
	if got := strings.Join(opened, " "); got != want {
		t.Errorf("opened %q, want %q", got, want)
	}
}

// A second click while an overlay opens is dropped: two popups can't
// open over one client.
func TestAClickWhileOpeningIsDropped(t *testing.T) {
	var opened int
	m := New(Config{Theme: theme.Default(), Open: func(string) { opened++ }})
	var tm tea.Model = m
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 26, Height: 12}, trainsMsg{trains: []Entry{{Handle: "reviewer-527192"}}}} {
		tm, _ = tm.Update(msg)
	}
	tm, first := tm.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft})
	tm, second := tm.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft})
	if first == nil || second != nil {
		t.Fatalf("first %v, second %v; want the first click only", first != nil, second != nil)
	}
	tm, _ = tm.Update(first())
	if _, again := tm.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft}); again == nil || opened != 1 {
		t.Errorf("after the overlay closed: a click opens again (%v), opened %d", again != nil, opened)
	}
}
