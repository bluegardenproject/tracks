package platform

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

func update(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func view(m tea.Model) string {
	lines, _ := m.(Model).layout()
	return escapes.ReplaceAllString(strings.Join(lines, "\n"), "")
}

// rowOf is the first row showing text.
func rowOf(t *testing.T, m tea.Model, text string) int {
	t.Helper()
	for i, l := range strings.Split(view(m), "\n") {
		if strings.Contains(l, text) {
			return i
		}
	}
	t.Fatalf("%q not shown:\n%s", text, view(m))
	return -1
}

var size = tea.WindowSizeMsg{Width: 26, Height: 20}

func TestPlatformShowsTheTrainsAndSignals(t *testing.T) {
	m := update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{
		trains: []Entry{
			{Handle: "reviewer-527192", Status: track.Done, Outcome: train.Blocked},
			{Handle: "watch-000001", Status: track.ActionRequired},
		},
		signals: Signals{RelayNew: 2, PRs: []PR{{Index: 0, PR: "PR #12", Action: "solve 3 comments"}, {Index: 1, PR: "PR #13", Action: "sent to the agent", Sent: true}}},
	})
	v := view(m)
	for _, want := range []string{"╭─ Platform ", "reviewer-527192", " done   blocked ", " action required ",
		"╭─ Signals ", "Relay Station · 2 new", "PR #12", " solve 3 comments ", "PR #13", "sent to the agent"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	lines := strings.Split(v, "\n")
	if len(lines) != 20 {
		t.Errorf("%d lines, want the pane's 20", len(lines))
	}
	for i, l := range lines {
		if w := len([]rune(l)); w != 26 {
			t.Errorf("line %d is %d wide, want 26: %q", i, w, l)
		}
	}
}

func TestThePlatformShowsWithoutTrains(t *testing.T) {
	m := update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{})
	v := view(m)
	lines := strings.Split(v, "\n")
	if !strings.HasPrefix(lines[0], "╭─ Platform ") || !strings.Contains(lines[1], "│ Trains ") || !strings.Contains(lines[2], "│ empty ") ||
		!strings.Contains(lines[4], "│ Side Tracks ") || !strings.Contains(lines[5], "│ empty ") || !strings.HasPrefix(lines[10], "╭─ Signals ") {
		t.Errorf("without trains or side tracks, the Platform says empty under each heading, over Signals, each half the pane:\n%s", v)
	}
	if !strings.Contains(lines[11], "│ Relay Station ") {
		t.Errorf("without messages, Signals still shows the Relay Station:\n%s", v)
	}
	if e := view(update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{err: errors.New("the daemon is gone")})); !strings.Contains(e, "Couldn't read the") || strings.Contains(e, "empty") {
		t.Errorf("an error:\n%s", e)
	}
}

// The frames' titles take their border's colour, and the trains, the
// Relay Station and the PRs the Station's list items' colour; "empty"
// is muted.
func TestThePlatformsColours(t *testing.T) {
	m := update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{signals: Signals{PRs: []PR{{PR: "PR #12", Action: "solve 1 comment"}}}})
	raw := strings.Join(func() []string { l, _ := m.(Model).layout(); return l }(), "\n")
	p := m.(Model).palette
	fg := func(tok theme.Token, s string) string { return lipgloss.NewStyle().Foreground(p.Color(tok)).Render(s) }
	for _, want := range []string{fg(theme.BorderDefault, "Platform"), fg(theme.BorderDefault, "Signals"), fg(theme.TextMuted, "empty"),
		fg(theme.TableTextDefault, "Relay Station"), fg(theme.TableTextDefault, "PR #12")} {
		if !strings.Contains(raw, want) {
			t.Errorf("missing %q in\n%q", want, raw)
		}
	}
	m = update(t, m, loadedMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done}}})
	raw = strings.Join(func() []string { l, _ := m.(Model).layout(); return l }(), "\n")
	if want := fg(theme.TableTextDefault, "reviewer-1"); !strings.Contains(raw, want) {
		t.Errorf("a train's handle isn't in the Station's list colour: %q", raw)
	}
}

func TestClicks(t *testing.T) {
	var did []string
	c := Config{
		Theme:            theme.Default(),
		Collect:          func() error { did = append(did, "collect"); return errors.New("answer it first") },
		OpenRelayStation: func() error { did = append(did, "station"); return nil },
		Solve:            func(i int) error { did = append(did, "solve "+string(rune('0'+i))); return nil },
	}
	m := update(t, New(c), size, loadedMsg{
		trains:  []Entry{{Handle: "reviewer-1", Status: track.Done, Held: true}},
		signals: Signals{PRs: []PR{{Index: 1, PR: "PR #12", Action: "solve 3 comments"}}},
	})
	click := func(x int, text string) tea.Cmd {
		t.Helper()
		_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: rowOf(t, m, text), Button: tea.MouseLeft})
		return cmd
	}
	for _, tc := range []struct {
		text string
		want string
	}{{"Collect messages", "collect"}, {"Relay Station", "station"}, {"solve 3 comments", "solve 1"}} {
		did = nil
		cmd := click(4, tc.text)
		if cmd == nil {
			t.Fatalf("a click on %q did nothing", tc.text)
		}
		m = update(t, m, cmd())
		if len(did) != 1 || did[0] != tc.want {
			t.Errorf("a click on %q did %v, want %s", tc.text, did, tc.want)
		}
	}
	for _, x := range []int{1, 24} {
		if click(x, "Collect messages") != nil {
			t.Errorf("a click at x=%d, beside the button, did something", x)
		}
	}
	if click(4, "reviewer-1") != nil {
		t.Error("a click on the train's name did something")
	}
}

func TestCollectUnderSignals(t *testing.T) {
	calls := 0
	m := update(t, New(Config{Theme: theme.Default(), Collect: func() error { calls++; return errors.New("answer it first") }}),
		size, loadedMsg{signals: Signals{Held: true}})
	v := view(m)
	signals := strings.Index(v, "Signals")
	if collect := strings.Index(v, "Collect messages"); collect < signals || collect < strings.Index(v, "Relay Station") {
		t.Fatalf("@github's held events: Collect messages under the Relay Station, in Signals:\n%s", v)
	}
	_, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, "Collect messages"), Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on it did nothing")
	}
	m = update(t, m, cmd())
	if calls != 1 || !strings.Contains(view(m), "answer it first") {
		t.Errorf("collected %d times, notice shown: %v", calls, strings.Contains(view(m), "answer it first"))
	}
	m = update(t, m, loadedMsg{})
	if v := view(m); strings.Contains(v, "Collect messages") || strings.Contains(v, "answer it first") {
		t.Errorf("once delivered, the button and its notice stayed:\n%s", v)
	}
}

func TestClicksAreSafe(t *testing.T) {
	calls := 0
	m := update(t, New(Config{Theme: theme.Default(), Collect: func() error { calls++; return nil }}),
		loadedMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done, Held: true}}})
	if _, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: 3, Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click before the pane's size is known did something")
	}
	if _, cmd := update(t, m, tea.WindowSizeMsg{Width: 4, Height: 12}).Update(tea.MouseClickMsg{X: 1, Y: 3, Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click in a pane too small to draw did something")
	}
	m = update(t, m, size)
	row := rowOf(t, m, "Collect messages")
	m, first := m.Update(tea.MouseClickMsg{X: 4, Y: row, Button: tea.MouseLeft})
	if first == nil {
		t.Fatal("a click on the button did nothing")
	}
	if _, again := m.Update(tea.MouseClickMsg{X: 4, Y: row, Button: tea.MouseLeft}); again != nil {
		t.Error("a second click while the first is on its way did something")
	}
	m = update(t, m, actedMsg{err: errors.New("answer it first")})
	if !strings.Contains(view(m), "answer it first") {
		t.Error("why a click couldn't isn't shown")
	}
	m = update(t, m, loadedMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done}}})
	if strings.Contains(view(m), "answer it first") {
		t.Error("the notice of a button that's gone stayed")
	}
}

func TestALongHandleEndsInAnEllipsis(t *testing.T) {
	m := update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{trains: []Entry{{Handle: "a-very-long-prefix-for-trains-123456", Status: track.Active}}})
	if !strings.Contains(view(m), "a-very-long-prefix-fo…") {
		t.Errorf("a long handle:\n%s", view(m))
	}
}

func TestPlatformReadsAgainOnAChange(t *testing.T) {
	calls := 0
	m := New(Config{Theme: theme.Default(), Trains: func() ([]Entry, error) { calls++; return nil, nil }})
	_, cmd := m.Update(changedMsg{})
	if cmd == nil {
		t.Fatal("a change read nothing")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				c()
			}
		}
	}
	if calls != 1 {
		t.Errorf("a change read the trains %d times, want once", calls)
	}
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

func TestTheRelayStationLine(t *testing.T) {
	for n, want := range map[int]string{0: "│ Relay Station", 12: "│ Relay Station · 12 new", 150: "│ Relay Station · 99+"} {
		m := update(t, New(Config{Theme: theme.Default()}), size, loadedMsg{signals: Signals{RelayNew: n}})
		if !strings.Contains(view(m), want+" ") {
			t.Errorf("%d new: want %q in\n%s", n, want, view(m))
		}
	}
	opened := 0
	m := update(t, New(Config{Theme: theme.Default(), OpenRelayStation: func() error { opened++; return errors.New("no client") }}),
		size, loadedMsg{signals: Signals{RelayNew: 3}})
	row := rowOf(t, m, "Relay Station")
	m, cmd := m.Update(tea.MouseClickMsg{X: 20, Y: row, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on the count beside Relay Station did nothing")
	}
	m = update(t, m, cmd(), loadedMsg{signals: Signals{RelayNew: 3}})
	if opened != 1 || !strings.Contains(view(m), "no client") {
		t.Errorf("opened %d times; why it couldn't should stay shown:\n%s", opened, view(m))
	}
}

// A click on a train's handle or badges opens it; the border, the blank
// rows, Collect messages and below the list don't. A held train's
// button pushes the trains after it down a row.
func TestAClickOpensTheTrain(t *testing.T) {
	var opened []string
	m := New(Config{Theme: theme.Default(), Open: func(h string) { opened = append(opened, h) }})
	var tm tea.Model = m
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 26, Height: 20}, loadedMsg{trains: []Entry{
		{Handle: "reviewer-527192", Status: track.Active, Held: true}, {Handle: "watch-000001", Status: track.Done}}}} {
		tm, _ = tm.Update(msg)
	}
	click := func(y int) {
		t.Helper()
		var cmd tea.Cmd
		tm, cmd = tm.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		if cmd != nil {
			if msg := cmd(); msg != nil {
				if _, ok := msg.(actedMsg); ok {
					tm, _ = tm.Update(msg)
				}
			}
		}
	}
	// Row 1 is the Trains heading, 4 Collect messages.
	for _, y := range []int{0, 1, 2, 3, 5, 6, 7, 8, 11} {
		click(y)
	}
	for _, x := range []int{0, 25} {
		if _, cmd := tm.Update(tea.MouseClickMsg{X: x, Y: 2, Button: tea.MouseLeft}); cmd != nil {
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
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 26, Height: 12}, loadedMsg{trains: []Entry{{Handle: "reviewer-527192"}}}} {
		tm, _ = tm.Update(msg)
	}
	tm, first := tm.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft})
	tm, second := tm.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft})
	if first == nil || second != nil {
		t.Fatalf("first %v, second %v; want the first click only", first != nil, second != nil)
	}
	tm, _ = tm.Update(first())
	if _, again := tm.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft}); again == nil || opened != 1 {
		t.Errorf("after the overlay closed: a click opens again (%v), opened %d", again != nil, opened)
	}
}

// A Signals click's notice shows under Signals, not in the Platform's
// frame above it.
func TestASignalsNoticeShowsInSignals(t *testing.T) {
	m := update(t, New(Config{Theme: theme.Default(), OpenRelayStation: func() error { return errors.New("no client") }}), size,
		loadedMsg{trains: []Entry{{Handle: "reviewer-1", Status: track.Done}}})
	m, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, "Relay Station"), Button: tea.MouseLeft})
	m = update(t, m, cmd())
	v := view(m)
	if strings.Index(v, "no client") < strings.Index(v, "Signals") {
		t.Errorf("the Relay Station's notice shows above Signals:\n%s", v)
	}
}

// The Platform and Signals each take half the pane, Signals an odd row;
// a list longer than its half is cut, and its hidden rows don't click.
func TestHalfAndHalf(t *testing.T) {
	var opened []string
	m := update(t, New(Config{Theme: theme.Default(), Open: func(h string) { opened = append(opened, h) }}),
		tea.WindowSizeMsg{Width: 26, Height: 11}, loadedMsg{trains: []Entry{
			{Handle: "reviewer-527192", Status: track.Active}, {Handle: "watch-000001", Status: track.Done}}})
	lines := strings.Split(view(m), "\n")
	if len(lines) != 11 || !strings.HasPrefix(lines[4], "╰") || !strings.HasPrefix(lines[5], "╭─ Signals ") || !strings.HasPrefix(lines[10], "╰") {
		t.Fatalf("want the Platform in rows 0-4, Signals in 5-10:\n%s", view(m))
	}
	if strings.Contains(view(m), "watch-000001") {
		t.Errorf("the second train fits no more, yet shows:\n%s", view(m))
	}
	for y := range 11 {
		if _, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft}); cmd != nil {
			cmd()
		}
	}
	if got := strings.Join(opened, " "); got != "reviewer-527192 reviewer-527192" {
		t.Errorf("opened %q; want the first train's two rows only", got)
	}
}

// Side tracks show under their heading, after the trains, with the
// Station's badges; a click on one switches to its window, and why it
// couldn't shows.
func TestSideTracks(t *testing.T) {
	var opened []string
	m := update(t, New(Config{Theme: theme.Default(), OpenSide: func(id string) error {
		opened = append(opened, id)
		return errors.New("its window is closed")
	}}), tea.WindowSizeMsg{Width: 26, Height: 30}, loadedMsg{
		trains: []Entry{{Handle: "reviewer-527192", Status: track.Done}},
		sides: []Side{
			{ID: "a", Name: "relay-store", Status: track.Active},
			{ID: "b", Name: "relay-cli", Status: track.Done, PR: track.PRsOpen},
		},
	})
	v := view(m)
	trains, sides := strings.Index(v, "Trains"), strings.Index(v, "Side Tracks")
	if trains < 0 || sides < trains || strings.Index(v, "reviewer-527192") > sides || strings.Index(v, "relay-store") < sides {
		t.Fatalf("want the trains under Trains, then the side tracks under Side Tracks:\n%s", v)
	}
	if !strings.Contains(v, " done   PR open ") || strings.Contains(v, "empty") {
		t.Errorf("want relay-cli's badges, and no empty:\n%s", v)
	}
	for _, text := range []string{"relay-store", "relay-cli"} {
		next, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, text), Button: tea.MouseLeft})
		if cmd == nil {
			t.Fatalf("a click on %s did nothing", text)
		}
		m = update(t, next, cmd())
	}
	if strings.Join(opened, " ") != "a b" {
		t.Errorf("switched to %q, want a b", opened)
	}
	v = view(m)
	if n := strings.Count(v, "its window is closed"); n != 1 || strings.Index(v, "its window is closed") > strings.Index(v, "Signals") {
		t.Errorf("why it couldn't switch shows %d times, want once, under Side Tracks:\n%s", n, v)
	}
	if v := view(update(t, m, loadedMsg{})); strings.Contains(v, "its window is closed") {
		t.Errorf("with the side tracks gone, their notice stayed:\n%s", v)
	}
	if _, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, "Side Tracks"), Button: tea.MouseLeft}); cmd != nil {
		t.Error("a click on the heading did something")
	}
}

// A long status leaves a side track's PR badge a row of its own, not
// cut, and that row switches too.
func TestASideTracksLongStatus(t *testing.T) {
	var opened int
	m := update(t, New(Config{Theme: theme.Default(), OpenSide: func(string) error { opened++; return nil }}), size,
		loadedMsg{sides: []Side{{ID: "a", Name: "relay-cli", Status: track.ActionRequired, PR: track.PRsOpen}}})
	v := view(m)
	if !strings.Contains(v, " action required ") || !strings.Contains(v, "│   PR open ") {
		t.Fatalf("want the status, then the PR's badge on a row of its own:\n%s", v)
	}
	_, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, "PR open"), Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on the PR's row did nothing")
	}
	if cmd(); opened != 1 {
		t.Errorf("switched %d times, want once", opened)
	}
}

// A side track shows its main track under Main Track, instead of Side
// Tracks, and its button switches to the main track's window.
func TestBackToMainTrack(t *testing.T) {
	var opened []string
	m := update(t, New(Config{Theme: theme.Default(), OpenSide: func(id string) error {
		opened = append(opened, id)
		return nil
	}}), tea.WindowSizeMsg{Width: 26, Height: 30}, loadedMsg{
		sides: []Side{{ID: "main-1", Name: "api-cache", Main: true}},
	})
	v := view(m)
	if !strings.Contains(v, "Main Track") || !strings.Contains(v, "api-cache") || !strings.Contains(v, "Back to main track") ||
		strings.Contains(v, "Side Tracks") {
		t.Fatalf("want Main Track, its name and the button, and no Side Tracks:\n%s", v)
	}
	next, cmd := m.Update(tea.MouseClickMsg{X: 4, Y: rowOf(t, m, "Back to main track"), Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on Back to main track did nothing")
	}
	update(t, next, cmd())
	if strings.Join(opened, " ") != "main-1" {
		t.Errorf("switched to %q, want main-1", opened)
	}
}
