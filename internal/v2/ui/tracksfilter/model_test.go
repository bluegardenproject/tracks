package tracksfilter

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

var (
	tab    = tea.KeyPressMsg{Code: tea.KeyTab}
	down   = tea.KeyPressMsg{Code: tea.KeyDown}
	up     = tea.KeyPressMsg{Code: tea.KeyUp}
	space  = tea.KeyPressMsg{Code: tea.KeySpace}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	escape = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func typed(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

func run(f track.Filter, msgs ...tea.Msg) Model {
	var m tea.Model = New(theme.Default(), f)
	m, _ = m.Update(tea.WindowSizeMsg{Width: Width, Height: Height()})
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(Model)
}

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

func plain(m Model) string { return escapes.ReplaceAllString(m.render(), "") }

func TestTicksAndPresets(t *testing.T) {
	// done (third), PR merged (below it), archived only, Last 7 days.
	right := tea.KeyPressMsg{Code: tea.KeyRight}
	m := run(track.Filter{}, tab, tab, space, down, space, down, space, down, right, right, space, enter)
	action, f := m.Result()
	want := track.Filter{Statuses: []string{"done"}, PRStatuses: []string{"merged"}, Archived: true, Started: track.Last7Days}
	if action != Apply || !reflect.DeepEqual(f, want) {
		t.Errorf("Result = %v, %+v; want Apply, %+v\n%s", action, f, want, plain(m))
	}
}

func TestShowsTheFilterOn(t *testing.T) {
	on := track.Filter{Statuses: []string{"active", "done"}, PRStatuses: []string{"none"}, Started: track.Between, From: "2026-09-01", To: "2026-09-20"}
	m := run(on)
	view := plain(m)
	for _, want := range []string{"[x] active", "[ ] action required", "[x] done", "[x] no PR", "[ ] Archived only", "(•) Between", "2026-09-01", "2026-09-20"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "] closed") {
		t.Errorf("closed is only archived tracks, which Archived only picks:\n%s", view)
	}
	if _, f := run(on, enter).Result(); !reflect.DeepEqual(f, on) {
		t.Errorf("applied unchanged = %+v, want %+v", f, on)
	}
}

func TestDates(t *testing.T) {
	// Typing in From picks Between; Tab moves on to To.
	m := run(track.Filter{}, append(append([]tea.Msg{down, down, down, down, tea.KeyPressMsg{Code: tea.KeyRight}}, typed("2026-09-10")...), tab)...)
	m = run2(m, typed("2026-09-01")...)
	m = run2(m, enter)
	if action, _ := m.Result(); action != Cancel || m.err != "From is after To." || !strings.Contains(plain(m), "From is after To.") {
		t.Fatalf("From after To: %v, err %q\n%s", action, m.err, plain(m))
	}
	for range 10 {
		m = run2(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m = run2(m, typed("2026-09-30")...)
	m = run2(m, enter)
	action, f := m.Result()
	if want := (track.Filter{Started: track.Between, From: "2026-09-10", To: "2026-09-30"}); action != Apply || !reflect.DeepEqual(f, want) {
		t.Errorf("Result = %v, %+v; want %+v", action, f, want)
	}

	m = run(track.Filter{}, append([]tea.Msg{down, down, down, down, tea.KeyPressMsg{Code: tea.KeyRight}}, append(typed("09/10"), enter)...)...)
	if action, _ := m.Result(); action != Cancel || m.err != "From isn't a date like 2026-09-29." {
		t.Errorf("a bad date: %v, err %q", action, m.err)
	}
}

func run2(m Model, msgs ...tea.Msg) Model {
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(Model)
}

func TestClearAndCancel(t *testing.T) {
	on := track.Filter{Archived: true}
	if action, _ := run(on, escape).Result(); action != Cancel {
		t.Errorf("Esc: %v", action)
	}
	m := run(on)
	for range len(controls) - 2 {
		m = run2(m, tab)
	}
	if action, _ := run2(m, enter).Result(); action != Clear {
		t.Errorf("Enter on Clear: %v", action)
	}
	if action, _ := run2(m, tab, enter).Result(); action != Cancel {
		t.Errorf("Enter on Cancel: %v", action)
	}
	if run2(m, up).focus == m.focus {
		t.Error("↑ from the buttons stayed put")
	}
}

func TestClicks(t *testing.T) {
	m := run(track.Filter{})
	click := func(label string) {
		t.Helper()
		for y, line := range strings.Split(plain(m), "\n") {
			if x := strings.Index(line, label); x >= 0 {
				m = run2(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
				return
			}
		}
		t.Fatalf("%q not drawn:\n%s", label, plain(m))
	}
	click("action required")
	click("Today")
	click("Apply")
	action, f := m.Result()
	if want := (track.Filter{Statuses: []string{"action_required"}, Started: track.Today}); action != Apply || !reflect.DeepEqual(f, want) {
		t.Errorf("Result = %v, %+v; want %+v", action, f, want)
	}
}
