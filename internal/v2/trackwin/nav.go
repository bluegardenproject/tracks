package trackwin

import (
	"slices"
	"strconv"
)

// Moves between windows. Window 0 is the Tracks window; every other
// window is a track, numbered by its window index.
const (
	MoveFirst = "first"
	MovePrev  = "prev"
	MoveNext  = "next"
	MoveLast  = "last"
)

// Destination returns the window that move leads to from window
// current. move is one of the moves or a window number. The moves skip
// the Tracks window and don't wrap around: ok is false when there's
// nowhere to go.
func Destination(windows []int, current int, move string) (to int, ok bool) {
	var tracks []int
	for _, w := range windows {
		if w != 0 {
			tracks = append(tracks, w)
		}
	}
	slices.Sort(tracks)
	if len(tracks) == 0 && move != "0" {
		return 0, false
	}
	switch move {
	case MoveFirst:
		return tracks[0], true
	case MoveLast:
		return tracks[len(tracks)-1], true
	case MoveNext:
		for _, w := range tracks {
			if w > current {
				return w, true
			}
		}
		return 0, false
	case MovePrev:
		if current == 0 {
			return tracks[len(tracks)-1], true
		}
		for _, w := range slices.Backward(tracks) {
			if w < current {
				return w, true
			}
		}
		return 0, false
	}
	n, err := strconv.Atoi(move)
	if err != nil || !slices.Contains(windows, n) {
		return 0, false
	}
	return n, true
}

// Switch makes move in session from window current. In a track window
// it lands on the agent pane.
func Switch(t Tmux, session, move string, current int) error {
	windows, err := t.ListWindows(session)
	if err != nil {
		return err
	}
	indexes := make([]int, len(windows))
	for i, w := range windows {
		indexes[i] = w.Index
	}
	to, ok := Destination(indexes, current, move)
	if !ok {
		return nil
	}
	target := "=" + session + ":" + strconv.Itoa(to)
	panes, err := t.ListPanes(target)
	if err != nil {
		return err
	}
	// The agent pane first, so the window shows it right away.
	if agent, _ := layout(panes); agent != nil {
		if err := t.SelectPane(agent.ID); err != nil {
			return err
		}
	}
	return t.SelectWindow(target)
}
