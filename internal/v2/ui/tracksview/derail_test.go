package tracksview

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

func TestDerailAsksFirst(t *testing.T) {
	var lost []string
	var forced []bool
	m := withEnded(Config{
		Lost: func(string) ([]string, error) { return lost, nil },
		Derail: func(id string, force bool) ([]string, error) {
			forced = append(forced, force)
			return nil, nil
		},
	})
	if view := plainView(m); !strings.Contains(view, " Derail ") {
		t.Fatalf("an ended track lacks Derail:\n%s", view)
	}

	m = settle(m, key('d'))
	view := plainView(m)
	for _, want := range []string{"Derail rate-bug?", "deleted for good.", " Derail ", " Cancel ", "y derail", "n/Esc/Enter cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Derail should ask, missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "stays open on GitHub") {
		t.Errorf("a track without PRs mentions one:\n%s", view)
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.station.asking != nil || len(forced) != 0 {
		t.Fatalf("Enter should cancel Derail: derailed %v", forced)
	}
	m = settle(settle(m, key('d')), key('y'))
	if !slices.Equal(forced, []bool{false}) || m.station.notice.text != "Derailed rate-bug." {
		t.Fatalf("confirmed: derailed %v, hint row %q", forced, m.station.notice.text)
	}

	lost = []string{"web: 1 commit that exists nowhere else"}
	m = settle(m, key('d'))
	if view := plainView(m); !strings.Contains(view, lost[0]) || !strings.Contains(view, "y derail anyway") {
		t.Fatalf("Derail should list the work it would lose:\n%s", view)
	}
	m = press(t, m, "Derail anyway")
	if !slices.Equal(forced, []bool{false, true}) {
		t.Errorf("Derail anyway: derailed %v, want it forced", forced)
	}
}

func TestDerailMentionsAnOpenPR(t *testing.T) {
	m := withEnded(Config{Lost: func(string) ([]string, error) { return nil, nil }})
	tracks := slices.Clone(m.station.tracks)
	tracks[1].PRs = []source.PR{{Repo: "acme/web", Number: 12, State: "open", URL: "https://github.com/acme/web/pull/12"}}
	m = settle(update(m, tracksMsg{tracks: tracks}), key('d'))
	if view := plainView(m); !strings.Contains(view, "Its PR #12 stays open on GitHub.") {
		t.Errorf("Derail should say the PR stays:\n%s", view)
	}
}

func TestDerailOnlyEndedTracks(t *testing.T) {
	calls := 0
	lost := func(string) ([]string, error) {
		calls++
		return nil, nil
	}
	m := withEnded(Config{Lost: lost})
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if view := plainView(m); strings.Contains(view, " Derail ") {
		t.Errorf("an open track offers Derail:\n%s", view)
	}
	if m = settle(m, key('d')); calls != 0 || m.station.asking != nil {
		t.Error("d derailed an open track")
	}

	archived := []source.Track{{ID: "d", Name: "old", Kind: "work", Status: track.Closed, Archived: true}}
	m = update(New(Config{Version: "test", Theme: theme.Default(), Lost: lost}),
		tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: archived})
	if view := plainView(m); !strings.Contains(view, " Derail ") {
		t.Fatalf("an archived track lacks Derail:\n%s", view)
	}
	if m = settle(m, key('d')); calls != 1 || m.station.asking == nil {
		t.Error("d doesn't ask to derail an archived track")
	}
}
