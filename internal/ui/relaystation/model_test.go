package relaystation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/theme"
)

var (
	down   = tea.KeyPressMsg{Code: tea.KeyDown}
	up     = tea.KeyPressMsg{Code: tea.KeyUp}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	escape = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlB  = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

var now = time.Date(2026, 11, 24, 16, 30, 0, 0, time.Local)

func plain(m Model) string { return escapes.ReplaceAllString(m.render(), "") }

func run(w, h int, msgs ...tea.Msg) Model {
	var m tea.Model = New(theme.Default(), func() time.Time { return now })
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(Model)
}

func long(lines int) string {
	var b []string
	for i := range lines {
		b = append(b, fmt.Sprintf("line %d", i+1))
	}
	return strings.Join(b, "\n")
}

var relayLog = Loaded{
	Messages: []relay.Message{
		{ID: 1, From: relay.TracksHandle, Kind: relay.Notice, Body: "@reviewer-1 joined: Reviewer", At: now.Add(-25 * time.Hour)},
		{ID: 2, From: "reviewer-1", To: relay.TrackHandle, Kind: relay.Report, Body: long(10), At: now.Add(-time.Hour)},
		{ID: 3, From: relay.TrackHandle, To: "reviewer-1", Kind: relay.Question, Body: "Why?", At: now},
		{ID: 4, From: "reviewer-1", To: relay.TrackHandle, Kind: relay.Answer, Body: "Because.", At: now},
	},
	Waiting: []int64{4},
}

func TestShowsEveryMessage(t *testing.T) {
	out := plain(run(80, 30, relayLog))
	for _, want := range []string{
		"Relay Station · 4 messages",
		"Nov 23 15:30 @reviewer-1 joined: Reviewer",
		"15:30 @reviewer-1 → @track  report",
		"16:30 @track → @reviewer-1  question",
		"16:30 @reviewer-1 → @track  answer   not delivered",
		"line 3", "… 7 more lines · Enter opens",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "line 4") {
		t.Errorf("a long message isn't folded:\n%s", out)
	}
	if strings.Count(out, "not delivered") != 1 {
		t.Errorf("only the answer waits for its pointer:\n%s", out)
	}
}

func TestOpensTheNewestAndUnfoldsOnEnter(t *testing.T) {
	m := run(80, 30, relayLog)
	if m.selected != 3 {
		t.Fatalf("selected %d on opening, want the newest", m.selected)
	}
	m = run(80, 30, relayLog, up, up, up, down, enter)
	out := plain(m)
	if !strings.Contains(out, "line 10") || !strings.Contains(out, "Enter folds") {
		t.Errorf("Enter didn't open the report:\n%s", out)
	}
	if out := plain(run(80, 30, relayLog, up, up, enter, enter)); strings.Contains(out, "line 10") {
		t.Errorf("a second Enter didn't fold it:\n%s", out)
	}
}

func TestScrolls(t *testing.T) {
	m := run(60, 8, relayLog)
	if out := plain(m); !strings.Contains(out, "Because.") || strings.Contains(out, "joined") {
		t.Errorf("opens scrolled to the end:\n%s", out)
	}
	m = run(60, 8, relayLog, tea.KeyPressMsg{Code: tea.KeyHome})
	if out := plain(m); !strings.Contains(out, "joined") {
		t.Errorf("Home doesn't show the oldest:\n%s", out)
	}
	m, _ = updated(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.top != wheelStep {
		t.Errorf("the wheel scrolled to %d, want %d", m.top, wheelStep)
	}
	for range 20 {
		m, _ = updated(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if m.top != m.maxTop() {
		t.Errorf("the wheel scrolled past the end: %d, max %d", m.top, m.maxTop())
	}
}

func TestStaysWhereTheUserScrolled(t *testing.T) {
	m := run(60, 8, relayLog, tea.KeyPressMsg{Code: tea.KeyHome})
	more := relayLog
	more.Messages = append(append([]relay.Message{}, relayLog.Messages...),
		relay.Message{ID: 5, From: "reviewer-1", To: relay.TrackHandle, Kind: relay.Report, Body: "Later.", At: now})
	m, _ = updated(m, more)
	if m.top != 0 || m.msgs[m.selected].ID != 1 {
		t.Errorf("a new message moved the view: top %d, selected #%d", m.top, m.msgs[m.selected].ID)
	}
	m = run(60, 8, relayLog)
	m, _ = updated(m, more)
	if m.top != m.maxTop() || m.msgs[m.selected].ID != 5 {
		t.Errorf("at the end, a new message isn't followed: top %d of %d, selected #%d", m.top, m.maxTop(), m.msgs[m.selected].ID)
	}
}

func TestClickSelectsAndOpens(t *testing.T) {
	m := run(80, 30, relayLog, tea.KeyPressMsg{Code: tea.KeyHome})
	// Row 0 is the frame's top, row 1 the notice, row 3 the report's head.
	m, _ = updated(m, tea.MouseClickMsg{Button: tea.MouseLeft, Y: 3})
	if m.selected != 1 || !m.open[2] {
		t.Errorf("a click on the report: selected %d, open %v", m.selected, m.open)
	}
}

func TestCloses(t *testing.T) {
	for _, key := range []tea.Msg{escape, tea.KeyPressMsg{Code: 'q', Text: "q"}} {
		if _, cmd := updated(run(80, 30, relayLog), key); cmd == nil {
			t.Errorf("%v doesn't close it", key)
		}
	}
	// Ctrl+b Esc, as a train's overlay hides: the popup gets both keys.
	m, cmd := updated(run(80, 30, relayLog), ctrlB)
	if cmd != nil {
		t.Error("Ctrl+b alone closes it")
	}
	if _, cmd := updated(m, escape); cmd == nil {
		t.Error("Ctrl+b Esc doesn't close it")
	}
}

func TestStates(t *testing.T) {
	if out := plain(run(60, 10)); !strings.Contains(out, "Reading the relay") {
		t.Errorf("before the first load:\n%s", out)
	}
	if out := plain(run(60, 10, Loaded{})); !strings.Contains(out, "No messages") {
		t.Errorf("an empty relay:\n%s", out)
	}
	if out := plain(run(60, 10, Loaded{Err: errors.New("daemon gone")})); !strings.Contains(out, "daemon gone") {
		t.Errorf("an error:\n%s", out)
	}
	if out := plain(run(60, 10, Loaded{Messages: []relay.Message{{ID: 1, From: "x-1", To: "track", Kind: relay.Event,
		Body: "red\x1b[2J\x1b]0;title\x07\ttab", At: now}}})); strings.Contains(out, "\x1b") || !strings.Contains(out, "red[2J]0;title    tab") {
		t.Errorf("a message's control characters reach the screen:\n%q", out)
	}
}

func updated(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}
