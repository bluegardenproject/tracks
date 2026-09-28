package tracksview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// addTrackLabel is the framed button in an empty Tracks frame.
const addTrackLabel = "Add new Track"

// addTrackSpot is where Add new Track sits in the list's body, width
// by height cells: centred below the header.
func addTrackSpot(width, height int) (x, y, w int) {
	w = lipgloss.Width(addTrackLabel) + 4
	return max(0, (width-w)/2), 1 + max(0, (height-1-3)/2), w
}

// emptyList puts Add new Track in the list's lines, width by height
// cells.
func (m Model) emptyList(lines []string, width, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	x, y, _ := addTrackSpot(width, height)
	for i, l := range m.framedButton(addTrackLabel, m.station.hoverAdd) {
		if y+i < height {
			lines[y+i] = strings.Repeat(" ", x) + l
		}
	}
	return lines
}

// onAddTrack reports whether cell x, y of the window is on Add new
// Track.
func (m Model) onAddTrack(x, y int) bool {
	if m.tab != tabStation || len(m.station.tracks) > 0 || m.station.err != nil {
		return false
	}
	p := m.panes()
	bx, by, w := addTrackSpot(p.listWidth-4, m.contentHeight()-2)
	left, top := p.listX+2+bx, m.contentTop()+1+by
	return x >= left && x < left+w && y >= top && y < top+3
}

// openNewTrack opens the New track form, and reads the tracks again
// once it closes.
func (m Model) openNewTrack() tea.Cmd {
	if m.newTrack == nil {
		return nil
	}
	open := m.newTrack
	return func() tea.Msg {
		if err := open(); err != nil {
			return doneMsg{err: fmt.Errorf("Couldn't open the New track form: %w", err)}
		}
		return doneMsg{reload: true}
	}
}
