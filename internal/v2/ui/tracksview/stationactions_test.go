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
// and cleaned; rate-bug is selected.
func withEnded(c Config) Model {
	c.Version, c.Theme = "test", theme.Default()
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "open-one", Kind: "work", Status: track.Active,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/aaa111", Path: "/tmp/wt/a/web"}}},
		{ID: "b", Name: "rate-bug", Kind: "work", Status: track.Done, Cleanable: true,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/abc123", Path: "/tmp/wt/b/web"}}, Session: "s-2",
			Created: time.Date(2026, 9, 28, 10, 15, 0, 0, time.Local)},
		{ID: "c", Name: "old-fix", Kind: "work", Status: track.Closed,
			Repos: []source.Repo{{Name: "web", Branch: "tracks/def456", Removed: true}}},
	}
	return update(New(c), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks}, tea.KeyPressMsg{Code: tea.KeyDown})
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r} }

func TestEndedRows(t *testing.T) {
	checks, resumes := 0, 0
	m := withEnded(Config{
		Unsaved: func(string) ([]string, error) {
			checks++
			return nil, nil
		},
		Resume: func(string, bool, func(string)) ([]string, error) {
			resumes++
			return nil, nil
		},
	})
	view := plainView(m)
	for _, want := range []string{"work   done ", " Resume ", " Clean ", "Enter resume", "/tmp/wt/b/web"} {
		if !strings.Contains(view, want) {
			t.Errorf("ended track: missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, " End ") {
		t.Error("an ended track offers End")
	}

	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	view = plainView(m)
	if !strings.Contains(view, "work   closed ") || !strings.Contains(view, "removed") || strings.Contains(view, "/tmp/wt") {
		t.Errorf("cleaned track: the worktree should show as removed:\n%s", view)
	}
	if strings.Contains(view, "Enter resume") {
		t.Errorf("a cleaned track offers Enter to resume:\n%s", view)
	}
	for _, k := range []tea.KeyPressMsg{key('l'), key('r'), {Code: tea.KeyEnter}} {
		if m = settle(m, k); checks != 0 || resumes != 0 || m.station.asking != nil {
			t.Errorf("%s ran Clean or Resume on a cleaned track", k)
		}
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
}

func TestCleanAsksFirst(t *testing.T) {
	var unsaved []string
	var forced []bool
	m := withEnded(Config{
		Unsaved: func(string) ([]string, error) { return unsaved, nil },
		Clean: func(id string, force bool) ([]string, error) {
			forced = append(forced, force)
			return nil, nil
		},
	})

	m = settle(m, key('l'))
	if view := plainView(m); !strings.Contains(view, "Remove the worktrees of rate-bug?") || !strings.Contains(view, " Remove ") {
		t.Fatalf("Clean should ask once:\n%s", view)
	}
	if m = settle(m, key('n')); m.station.asking != nil || len(forced) != 0 {
		t.Fatal("cancelling cleaned anyway")
	}
	m = settle(settle(m, key('l')), key('y'))
	if !slices.Equal(forced, []bool{false}) || m.station.notice.text != "Removed the worktrees of rate-bug." {
		t.Fatalf("confirmed: cleaned %v, hint row %q", forced, m.station.notice.text)
	}

	unsaved = []string{"web: 3 changed files"}
	m = settle(m, key('l'))
	if view := plainView(m); !strings.Contains(view, "web: 3 changed files") || !strings.Contains(view, "y remove anyway") {
		t.Fatalf("Clean should list the unsaved work:\n%s", view)
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter}); len(forced) != 1 || m.station.asking != nil {
		t.Fatalf("Enter should cancel removing unsaved work: cleaned %v", forced)
	}
	m = settle(m, key('l'))
	m = press(t, m, "Remove anyway")
	if !slices.Equal(forced, []bool{false, true}) {
		t.Errorf("Remove anyway: cleaned %v, want it forced", forced)
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
