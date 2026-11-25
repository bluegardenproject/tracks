package tracksview

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

var t0 = time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)

func listed(id string, minute int) source.Track {
	return source.Track{ID: id, Name: id, Kind: "Work", Status: track.Active, Created: t0.Add(time.Duration(minute) * time.Minute)}
}

func sideOf(id string, minute int, main source.Track) source.Track {
	t := listed(id, minute)
	t.Side, t.ParentID, t.Parent = "side-"+id, main.ID, main.Name
	return t
}

func names(tracks []source.Track) string {
	var out []string
	for _, t := range tracks {
		n := t.ID
		if t.Under {
			n = "└" + n
		}
		out = append(out, n)
	}
	return strings.Join(out, " ")
}

func TestGroup(t *testing.T) {
	api, docs := listed("api", 0), listed("docs", 2)
	archivedMain := listed("old", 1)
	archivedMain.Archived = true
	started := listed("child", 7)
	started.ParentID, started.Parent = api.ID, api.Name
	for _, c := range []struct {
		name   string
		tracks []source.Track
		want   string
	}{
		{"no side tracks", []source.Track{api, docs}, "api docs"},
		{"side tracks right after their main track, oldest first",
			[]source.Track{api, docs, sideOf("cli", 5, api), sideOf("store", 3, api)},
			"api └store └cli docs"},
		{"in the main track's order, whatever the list's",
			[]source.Track{docs, api, sideOf("d1", 4, docs), sideOf("a1", 3, api)},
			"docs └d1 api └a1"},
		{"a side track whose main track is filtered out keeps its place",
			[]source.Track{docs, sideOf("cli", 5, api)}, "docs cli"},
		{"a side track whose main track is archived shows on its own",
			[]source.Track{archivedMain, docs, sideOf("cli", 5, archivedMain)}, "old docs cli"},
		{"archived with its archived main track",
			[]source.Track{archivedMain, docs, func() source.Track {
				s := sideOf("cli", 5, archivedMain)
				s.Archived = true
				return s
			}()}, "old └cli docs"},
		{"a main track gone: no parent", []source.Track{api, func() source.Track {
			s := sideOf("cli", 5, api)
			s.ParentID, s.Parent = "", ""
			return s
		}()}, "api cli"},
		{"a track started from another isn't a side track", []source.Track{api, docs, started}, "api docs child"},
		{"no nesting under a side track", []source.Track{api, sideOf("s1", 1, api), sideOf("s2", 2, sideOf("s1", 1, api))},
			"api └s1 s2"},
	} {
		got := group(c.tracks)
		if names(got) != c.want {
			t.Errorf("%s: got %q, want %q", c.name, names(got), c.want)
		}
		if len(got) != len(c.tracks) {
			t.Errorf("%s: %d tracks out of %d", c.name, len(got), len(c.tracks))
		}
	}
}

func groupedStation(t *testing.T, w int, opened *[]int) Model {
	t.Helper()
	api, docs := listed("api-cache", 0), listed("docs-fix", 9)
	api.Number, docs.Number = 1, 4
	store, cli := sideOf("relay-store", 3, api), sideOf("relay-cli", 5, api)
	store.Number, cli.Number = 2, 3
	return withTracks([]source.Track{api, docs, cli, store}, w, 40, opened)
}

func TestStationGroupsSideTracks(t *testing.T) {
	var opened []int
	m := groupedStation(t, 160, &opened)
	view := plainView(m)
	order := []string{"api-cache", "└ side: relay-store", "└ side: relay-cli", "docs-fix"}
	last := -1
	for _, want := range order {
		i := strings.Index(view, want)
		if i < 0 || i < last {
			t.Fatalf("want rows in order %q:\n%s", order, view)
		}
		last = i
	}

	narrow := plainView(update(m, tea.WindowSizeMsg{Width: 44, Height: 40}))
	if !strings.Contains(narrow, "└ side: re") || strings.Contains(narrow, "relay-store") {
		t.Errorf("a narrow list should cut the name and keep the corner and prefix:\n%s", narrow)
	}
	for width, want := range map[int]string{8: "└ side:…", 7: "└ side…"} {
		if got := escapes.ReplaceAllString(m.sideName(m.station.tracks[1], width, false, lipgloss.NewStyle()), ""); got != want {
			t.Errorf("%d cells: %q, want %q", width, got, want)
		}
	}
}

func TestStationGroupedRowsSelectAndOpen(t *testing.T) {
	var opened []int
	m := groupedStation(t, 160, &opened)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.station.tracks[m.station.selected].ID; got != "relay-store" {
		t.Fatalf("down from the main track selected %s, want its first side track", got)
	}
	if !strings.Contains(plainView(m), "Side track of api-cache") {
		t.Error("a side track's details should name its main track")
	}
	m = run(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !slices.Equal(opened, []int{2}) {
		t.Errorf("Enter on relay-store opened %v, want window 2", opened)
	}

	y := m.stationTop() + 2
	m = run(m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	m = run(m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	if got := m.station.tracks[m.station.selected].ID; got != "relay-cli" || !slices.Equal(opened, []int{2, 3}) {
		t.Errorf("double click on the third row: selected %s, opened %v; want relay-cli, window 3", got, opened)
	}

	m = update(m, tracksMsg{tracks: append([]source.Track{sideOf("relay-new", 1, listed("api-cache", 0))}, m.station.tracks...)})
	if got := m.station.tracks[m.station.selected].ID; got != "relay-cli" {
		t.Errorf("after a reload that moved the rows the selection is %s, want relay-cli still", got)
	}
}
