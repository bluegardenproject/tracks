package tracksview

import (
	"cmp"
	"slices"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/termtext"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// group puts each side track right after its main track, side tracks
// among themselves oldest first, and marks them Under. A side track
// whose main track isn't listed with it (filtered out, archived, gone)
// keeps its place, on its own.
func group(tracks []source.Track) []source.Track {
	at := make(map[string]int, len(tracks))
	for i, t := range tracks {
		if t.ID != "" {
			at[t.ID] = i
		}
	}
	under := func(t source.Track) bool {
		if t.Side == "" || t.ParentID == "" || t.ParentID == t.ID {
			return false
		}
		i, ok := at[t.ParentID]
		return ok && tracks[i].Side == "" && tracks[i].Archived == t.Archived
	}
	sides := map[string][]source.Track{}
	for _, t := range tracks {
		if under(t) {
			t.Under = true
			sides[t.ParentID] = append(sides[t.ParentID], t)
		}
	}
	out := make([]source.Track, 0, len(tracks))
	for _, t := range tracks {
		if under(t) {
			continue
		}
		t.Under = false
		out = append(out, t)
		s := sides[t.ID]
		if t.ID == "" || len(s) == 0 {
			continue
		}
		slices.SortStableFunc(s, func(a, b source.Track) int {
			return cmp.Or(a.Created.Compare(b.Created), cmp.Compare(a.ID, b.ID))
		})
		out = append(out, s...)
	}
	return out
}

// sidePrefix goes before a side track's name under its main track.
const (
	sideCorner = "└ "
	sidePrefix = "side: "
)

// shownInList is t's name as the list shows it.
func shownInList(t source.Track) string {
	shown := termtext.SingleLine(t.Shown())
	if t.Under {
		return sideCorner + sidePrefix + shown
	}
	return shown
}

// sideName draws side track t's name under its main track, at most
// width cells: the corner and prefix stay, the name is cut.
func (m Model) sideName(t source.Track, width int, selected bool, fill lipgloss.Style) string {
	lead := lipgloss.Width(sideCorner + sidePrefix)
	muted := fill.Foreground(m.palette.Color(theme.TableTextMuted))
	if width <= lead {
		return muted.Render(cut(shownInList(t), width))
	}
	name := fill.Foreground(m.palette.Color(theme.TableTextDefault)).Bold(selected)
	return muted.Render(sideCorner+sidePrefix) + name.Render(cut(termtext.SingleLine(t.Shown()), width-lead))
}
