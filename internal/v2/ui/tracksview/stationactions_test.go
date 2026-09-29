package tracksview

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// withEnded is an open track, then rate-bug, ended, and old-fix, ended
// and unarchived after Archive removed its worktree; rate-bug is
// selected.
func withEnded(c Config) Model {
	c.Version, c.Theme = "test", theme.Default()
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "open-one", Kind: "work", Status: track.Active,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/aaa111", Path: "/tmp/wt/a/web"}}},
		{ID: "b", Name: "rate-bug", Kind: "work", Status: track.Done, Removable: true,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/abc123", Path: "/tmp/wt/b/web"}}, Session: "s-2",
			Created: time.Date(2026, 9, 28, 10, 15, 0, 0, time.Local)},
		{ID: "c", Name: "old-fix", Kind: "work", Status: track.Done,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/def456", Removed: true}}},
	}
	return update(New(c), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks}, tea.KeyPressMsg{Code: tea.KeyDown})
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r} }

func TestEndedRows(t *testing.T) {
	checks, resumes := 0, 0
	m := withEnded(Config{
		Lost: func(string) ([]string, error) {
			checks++
			return nil, nil
		},
		Resume: func(string, bool, func(string)) ([]string, error) {
			resumes++
			return nil, nil
		},
	})
	view := plainView(m)
	for _, want := range []string{"work   done ", " Resume ", " Archive ", "Enter resume", "/tmp/wt/b/web"} {
		if !strings.Contains(view, want) {
			t.Errorf("ended track: missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, " End ") || strings.Contains(view, " Clean ") {
		t.Errorf("an ended track offers End or Clean:\n%s", view)
	}

	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	view = plainView(m)
	if !strings.Contains(view, "work   done ") || !strings.Contains(view, "removed") || strings.Contains(view, "/tmp/wt") {
		t.Errorf("unarchived track: done, with the worktree shown as removed:\n%s", view)
	}
	if !strings.Contains(view, "Enter resume") {
		t.Errorf("an unarchived track can't be resumed:\n%s", view)
	}
	if m = settle(m, key('l')); checks != 0 || m.station.asking != nil {
		t.Error("l still does something")
	}
	if m = settle(m, key('r')); resumes != 1 {
		t.Errorf("r resumed %d times, want once", resumes)
	}
}

func TestResumeShowsProgress(t *testing.T) {
	var resumed []string
	m := withEnded(Config{Resume: func(id string, _ bool, progress func(string)) ([]string, error) {
		resumed = append(resumed, id)
		progress("Starting Claude Code…")
		return nil, nil
	}})
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	notices := []string{m.station.notice.text}
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(Model)
		notices = append(notices, m.station.notice.text)
	}
	want := []string{"Resuming rate-bug…", "Starting Claude Code…", "Resumed rate-bug."}
	if !slices.Equal(notices, want) || !slices.Equal(resumed, []string{"b"}) {
		t.Errorf("resumed %v, hint row %q; want b and %q", resumed, notices, want)
	}

	for err, want := range map[error]string{
		tracks.Problem("Add Cursor on the Engines tab to resume this track."): "Add Cursor on the Engines tab to resume this track.",
		errors.New("tmux is gone"): "Couldn't resume rate-bug: tmux is gone",
	} {
		m := withEnded(Config{Resume: func(string, bool, func(string)) ([]string, error) { return nil, err }})
		if m = settle(m, key('r')); m.station.notice != (notice{text: want, err: true}) {
			t.Errorf("failed resume: hint row %+v, want %q", m.station.notice, want)
		}
	}
}

func TestResumeAsksForMissingWorktree(t *testing.T) {
	var recreated []bool
	m := withEnded(Config{Resume: func(_ string, recreate bool, _ func(string)) ([]string, error) {
		recreated = append(recreated, recreate)
		if !recreate {
			return []string{"web: /tmp/wt/b/web"}, nil
		}
		return nil, nil
	}})

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	view := plainView(m)
	for _, want := range []string{"Worktree couldn't be found", "web: /tmp/wt/b/web", " Re-create worktree ", " Cancel ", "y re-create"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.station.asking != nil || !slices.Equal(recreated, []bool{false}) {
		t.Fatalf("Enter should cancel: resumed %v", recreated)
	}
	m = press(t, settle(m, key('r')), "Re-create worktree")
	if !slices.Equal(recreated, []bool{false, false, true}) || m.station.notice.text != "Resumed rate-bug." {
		t.Errorf("re-create: resumed %v, hint row %q", recreated, m.station.notice.text)
	}
	if strings.Contains(view, "deleted branch") {
		t.Errorf("rate-bug kept its branch, but the question says it's deleted:\n%s", view)
	}

	// Archive removed old-fix's branch too.
	m = settle(settle(m, tea.KeyPressMsg{Code: tea.KeyDown}), key('r'))
	if view := plainView(m); !strings.Contains(view, "A deleted branch comes back from origin,") {
		t.Errorf("old-fix's question should say where its branch comes from:\n%s", view)
	}
}

func TestStatusBadgesAreThemeTokens(t *testing.T) {
	for _, s := range slices.Concat(track.Statuses, track.PRStatuses) {
		bg, text := badgeTokens(s)
		if !slices.Contains(theme.All, bg) || !slices.Contains(theme.All, text) {
			t.Errorf("%s's badge %q isn't a theme state", s.ID, s.Badge)
		}
		if want := theme.StateInfoBg; s == track.ActionRequired {
			if bg != theme.StateWarningBg || text != theme.StateWarningText {
				t.Errorf("action required is drawn in %s on %s, want the warning badge", text, bg)
			}
		} else if bg != want || text != theme.StateInfoText {
			t.Errorf("%s is drawn in %s on %s, want the info badge", s.ID, text, bg)
		}
	}
}
