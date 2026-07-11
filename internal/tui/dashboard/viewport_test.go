package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// makeModel builds a dashboard model with n running tracks for layout
// tests. IDs are "track-000".."track-NNN" so shortID keeps them intact
// and they're easy to search for in rendered output.
func makeModel(n, width, height int) *model {
	tracks := make([]state.Track, n)
	for i := range tracks {
		tracks[i] = state.Track{
			ID:     fmt.Sprintf("track-%03d", i),
			Branch: fmt.Sprintf("feat/branch-%d", i),
			Slug:   fmt.Sprintf("slug-%d", i),
			Status: state.StatusRunning,
			Kind:   state.KindWork,
		}
	}
	return &model{
		styles: defaultStyles(),
		tracks: tracks,
		width:  width,
		height: height,
	}
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// TestViewNeverExceedsHeight is the regression guard for the selection
// bug: a frame taller than the terminal makes Bubble Tea garble the
// table and detach the highlight. View must fit within m.height for any
// track count, cursor position, and whether the detail panel is shown.
func TestViewNeverExceedsHeight(t *testing.T) {
	heights := []int{10, 20, 24, 40, 60}
	counts := []int{0, 1, 5, 50, 500}
	for _, h := range heights {
		for _, n := range counts {
			for _, withDetail := range []bool{false, true} {
				m := makeModel(n, 120, h)
				if withDetail && n > 0 {
					m.cursor = n / 2
					m.detail = &detail{track: m.tracks[m.cursor]}
				}
				// exercise cursor extremes too
				for _, cur := range []int{0, n / 2, n - 1} {
					if cur < 0 {
						cur = 0
					}
					m.cursor = cur
					if got := lineCount(m.View()); got > h {
						t.Errorf("n=%d h=%d detail=%v cursor=%d: View is %d lines, exceeds height",
							n, h, withDetail, cur, got)
					}
				}
			}
		}
	}
}

// TestSelectedRowAlwaysVisible confirms the scrolling window keeps the
// cursor's track on screen no matter where the cursor sits in a long
// list — the user can always see what they're about to act on.
func TestSelectedRowAlwaysVisible(t *testing.T) {
	m := makeModel(100, 120, 24)
	for _, cur := range []int{0, 1, 25, 50, 98, 99} {
		m.cursor = cur
		out := m.View()
		want := m.tracks[cur].ID // shortID leaves 9-char IDs intact
		if !strings.Contains(out, want) {
			t.Errorf("cursor=%d: selected track %q not visible in frame", cur, want)
		}
	}
}

// TestScrollIndicatorsShown checks the "N more" affordances appear when
// the list is scrolled, and not when everything fits.
func TestScrollIndicatorsShown(t *testing.T) {
	// Long list, cursor in the middle → both indicators.
	m := makeModel(100, 120, 24)
	m.cursor = 50
	out := m.View()
	if !strings.Contains(out, "more") {
		t.Errorf("expected a scroll indicator with a long list, got none:\n%s", out)
	}

	// Everything fits → no indicators.
	small := makeModel(3, 120, 40)
	if strings.Contains(small.View(), "more") {
		t.Errorf("did not expect scroll indicators when all rows fit")
	}
}

// TestCursorReanchorsByID guards the other half of the fix: when a poll
// returns a track list that grew, shrank, or reordered, the selection
// must stay on the *same track*, not the same row index — otherwise
// end/kill/forget silently act on whatever slid under the cursor.
func TestCursorReanchorsByID(t *testing.T) {
	ids := func(names ...string) []state.Track {
		out := make([]state.Track, len(names))
		for i, n := range names {
			out[i] = state.Track{ID: n, Status: state.StatusRunning, Kind: state.KindWork}
		}
		return out
	}

	cases := []struct {
		name    string
		start   []state.Track
		cursor  int
		next    []state.Track
		wantID  string // expected selected ID after the poll
		wantCur int
	}{
		{
			name:    "new track appended keeps selection",
			start:   ids("a", "b", "c"),
			cursor:  1, // b
			next:    ids("a", "b", "c", "d"),
			wantID:  "b",
			wantCur: 1,
		},
		{
			name:    "track removed above cursor shifts index but keeps track",
			start:   ids("a", "b", "c"),
			cursor:  2, // c
			next:    ids("b", "c"),
			wantID:  "c",
			wantCur: 1,
		},
		{
			name:    "reordered list follows the track",
			start:   ids("a", "b", "c"),
			cursor:  0, // a
			next:    ids("c", "b", "a"),
			wantID:  "a",
			wantCur: 2,
		},
		{
			name:    "selected track removed clamps into range",
			start:   ids("a", "b", "c"),
			cursor:  1, // b
			next:    ids("a", "c"),
			wantID:  "", // b is gone; just require a valid cursor
			wantCur: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &model{styles: defaultStyles(), tracks: c.start, cursor: c.cursor}
			m.Update(pollResult{tracks: c.next})
			if m.cursor < 0 || m.cursor >= len(m.tracks) {
				t.Fatalf("cursor %d out of range for %d tracks", m.cursor, len(m.tracks))
			}
			if m.cursor != c.wantCur {
				t.Errorf("cursor = %d, want %d", m.cursor, c.wantCur)
			}
			if c.wantID != "" && m.tracks[m.cursor].ID != c.wantID {
				t.Errorf("selected %q, want %q", m.tracks[m.cursor].ID, c.wantID)
			}
		})
	}
}

func TestVisibleRowWindow(t *testing.T) {
	cases := []struct {
		n, cursor, cap     int
		wantStart, wantEnd int
	}{
		{5, 0, 10, 0, 5},       // capacity exceeds n → show all
		{5, 4, 5, 0, 5},        // exactly fits
		{100, 0, 10, 0, 10},    // cursor at top
		{100, 99, 10, 90, 100}, // cursor at bottom
		{100, 50, 10, 45, 55},  // centered
		{100, 3, 10, 0, 10},    // near top clamps to 0
		{100, 97, 10, 90, 100}, // near bottom clamps to end
	}
	for _, c := range cases {
		start, end := visibleRowWindow(c.n, c.cursor, c.cap)
		if start != c.wantStart || end != c.wantEnd {
			t.Errorf("visibleRowWindow(n=%d,cursor=%d,cap=%d) = (%d,%d), want (%d,%d)",
				c.n, c.cursor, c.cap, start, end, c.wantStart, c.wantEnd)
		}
		// The window must always contain the cursor.
		if c.cursor < start || c.cursor >= end {
			t.Errorf("visibleRowWindow(n=%d,cursor=%d,cap=%d): window [%d,%d) excludes cursor",
				c.n, c.cursor, c.cap, start, end)
		}
	}
}
