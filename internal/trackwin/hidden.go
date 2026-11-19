package trackwin

import (
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/tmux"
)

// BackSession holds the hidden panes, such as a track's reviewer: a
// session of its own, so its windows never count as tracks. A window
// of its own keeps it alive while every hidden pane is shown.
const BackSession = "tracks-back"

// keepWindow is BackSession's window that never closes: tmux ends a
// session with its last window.
const keepWindow = "keep"

// keepCommand is what keepWindow runs: nothing, for good.
const keepCommand = "sh -c 'while :; do sleep 3600; done'"

// paneTrackOption names the track a hidden pane belongs to.
const paneTrackOption = "@tracks_track"

// HiddenTmux is what hidden panes need from a tmux server beyond Tmux.
type HiddenTmux interface {
	Tmux
	HasSession(name string) bool
	NewSessionAt(name, window, dir, command string) error
	ListSessionPanes(session string) ([]tmux.Pane, error)
	JoinPane(src, target string, dir tmux.Split, percent int) error
	BreakPane(pane, session string) error
	KillPane(pane string) error
}

// ErrNoHidden means the track has no hidden pane in that role.
var ErrNoHidden = errors.New("no such pane")

// OpenHidden starts p in dir in a pane of its own in BackSession,
// labelled role and title for track, and returns its ID. It's shown in
// the track's window with Show.
func OpenHidden(t HiddenTmux, track, role, dir string, p Process) (string, error) {
	if !t.HasSession(BackSession) {
		if err := t.NewSessionAt(BackSession, keepWindow, "", keepCommand); err != nil && !t.HasSession(BackSession) {
			return "", fmt.Errorf("start the session for hidden panes: %w", err)
		}
	}
	_, pane, err := t.NewWindow(BackSession, track+"-"+role, dir, p.Command)
	if err != nil {
		return "", err
	}
	if err := label(t, pane, role, p.Title); err != nil {
		return pane, err
	}
	return pane, t.SetPaneOption(pane, paneTrackOption, track)
}

// Hidden is track's pane in role, shown in window or hidden; shown says
// which. ErrNoHidden when there's none.
func Hidden(t HiddenTmux, track, window, role string) (pane tmux.Pane, shown bool, err error) {
	if window != "" {
		panes, err := t.ListPanes(window)
		if err != nil {
			return tmux.Pane{}, false, err
		}
		for _, p := range panes {
			if p.Role == role && p.Track == track {
				return p, true, nil
			}
		}
	}
	if !t.HasSession(BackSession) {
		return tmux.Pane{}, false, ErrNoHidden
	}
	panes, err := t.ListSessionPanes(BackSession)
	if err != nil {
		return tmux.Pane{}, false, err
	}
	for _, p := range panes {
		if p.Role == role && p.Track == track {
			return p, false, nil
		}
	}
	return tmux.Pane{}, false, ErrNoHidden
}

// Show moves hidden pane into the bottom of window's right column,
// running on as it was, and evens out the column.
func Show(t HiddenTmux, window, pane string) error {
	panes, err := t.ListPanes(window)
	if err != nil {
		return err
	}
	agent, column := layout(panes)
	switch {
	case agent == nil:
		return fmt.Errorf("window %s has no agent pane", window)
	case len(column) == 0:
		err = t.JoinPane(pane, agent.ID, tmux.Right, ColumnPercent)
	default:
		err = t.JoinPane(pane, column[len(column)-1].ID, tmux.Below, 0)
	}
	if err != nil {
		return err
	}
	return even(t, window)
}

// Hide moves pane out of window back into BackSession, running on, and
// evens out what's left of the column.
func Hide(t HiddenTmux, window, pane string) error {
	if !t.HasSession(BackSession) {
		if err := t.NewSessionAt(BackSession, keepWindow, "", keepCommand); err != nil && !t.HasSession(BackSession) {
			return fmt.Errorf("start the session for hidden panes: %w", err)
		}
	}
	if err := t.BreakPane(pane, BackSession); err != nil {
		return err
	}
	return even(t, window)
}

// CloseHidden ends track's panes in role, shown in window or hidden;
// "" for every role. Ending a track ends its hidden panes too.
func CloseHidden(t HiddenTmux, track, window, role string) error {
	var all []tmux.Pane
	if window != "" {
		if panes, err := t.ListPanes(window); err == nil {
			all = append(all, panes...)
		}
	}
	if t.HasSession(BackSession) {
		panes, err := t.ListSessionPanes(BackSession)
		if err != nil {
			return err
		}
		all = append(all, panes...)
	}
	var errs []error
	closed := false
	for _, p := range all {
		if p.Track == track && (role == "" || p.Role == role) {
			errs = append(errs, t.KillPane(p.ID))
			closed = true
		}
	}
	if closed && window != "" {
		errs = append(errs, even(t, window))
	}
	return errors.Join(errs...)
}

// TrackOf is the ID of the track window belongs to, "" for none.
func TrackOf(t Tmux, window string) string {
	id, _ := t.WindowOption(window, trackOption)
	return id
}
