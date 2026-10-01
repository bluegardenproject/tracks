package tracksview

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The notices' clocks would hold every test up for noticeFor.
func init() { noticeTick = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil } }

// clocks records the notices' clocks instead of starting them.
func clocks(t *testing.T) *[]tea.Msg {
	var ends []tea.Msg
	stopped := noticeTick
	noticeTick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		if d != noticeFor {
			t.Errorf("a notice's clock runs %v, want %v", d, noticeFor)
		}
		ends = append(ends, fn(time.Time{}))
		return nil
	}
	t.Cleanup(func() { noticeTick = stopped })
	return &ends
}

func hintRow(m Model) string {
	lines := strings.Split(plainView(m), "\n")
	return lines[len(lines)-1]
}

func TestDoneNoticesEnd(t *testing.T) {
	ends := clocks(t)
	var shown []string
	m := withEnded(Config{Resume: func(_ string, _ bool, progress func(string)) ([]string, error) {
		progress("Starting Claude Code…")
		return nil, nil
	}})
	next, cmd := m.Update(key('r'))
	for m = next.(Model); cmd != nil; m = next.(Model) {
		if row := hintRow(m); strings.Contains(row, "✕") || len(*ends) != 0 {
			t.Errorf("work under way should stay, without ✕: %q, %d clocks", row, len(*ends))
		}
		shown = append(shown, strings.TrimSpace(hintRow(m)))
		next, cmd = m.Update(cmd())
	}
	if len(shown) != 2 {
		t.Errorf("progress shown: %q", shown)
	}
	if row := hintRow(m); !strings.Contains(row, "Resumed rate-bug.  ✕") || len(*ends) != 1 {
		t.Fatalf("done: %q, %d clocks; want the notice with its ✕ and one clock", row, len(*ends))
	}

	m = settle(m, key('r'))
	if m = settle(m, (*ends)[0]); m.station.notice.text == "" {
		t.Fatal("an earlier notice's clock ended the latest one")
	}
	if m = settle(m, (*ends)[1]); m.station.notice.text != "" {
		t.Errorf("after %v: %q", noticeFor, hintRow(m))
	}
}

func TestClosingANotice(t *testing.T) {
	ends := clocks(t)
	m := withEnded(Config{Resume: func(string, bool, func(string)) ([]string, error) {
		return nil, errors.New("tmux is gone")
	}})
	m = settle(m, key('r'))
	row := hintRow(m)
	x := strings.Index(row, "✕")
	if !m.station.notice.err || x < 0 || len(*ends) != 0 {
		t.Fatalf("an error: %q, %d clocks; want its ✕ and no clock", row, len(*ends))
	}
	y := len(strings.Split(plainView(m), "\n")) - 1
	m = settle(m, tea.MouseClickMsg{X: len([]rune(row[:x])), Y: y, Button: tea.MouseLeft})
	if m.station.notice.text != "" {
		t.Errorf("clicking ✕ left %q", hintRow(m))
	}
}
