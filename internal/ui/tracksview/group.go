package tracksview

import (
	"cmp"
	"slices"

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
