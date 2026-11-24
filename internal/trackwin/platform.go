package trackwin

import (
	"fmt"
)

// RolePlatform is the Platform, the pane on a track window's left that
// lists its trains (see docs/design/trains.md). It isn't in the right
// column.
const RolePlatform = "platform"

// PlatformWidth is the Platform's width: its frame, and a badge as wide
// as "action required" inside.
const PlatformWidth = 26

// OpenPlatform opens window's Platform: p in a pane PlatformWidth wide
// on the window's left, full height, in dir. The agent pane keeps focus.
func OpenPlatform(t Tmux, window, dir string, p Process) (string, error) {
	panes, err := t.ListPanes(window)
	if err != nil {
		return "", err
	}
	agent, _ := layout(panes)
	if agent == nil {
		return "", fmt.Errorf("window %s has no agent pane", window)
	}
	pane, err := t.SplitLeft(agent.ID, PlatformWidth, dir, p.Command)
	if err != nil {
		return "", err
	}
	return pane, label(t, pane, RolePlatform, p.Title)
}

// Platform is window's Platform pane, "" for none.
func Platform(t Tmux, window string) (string, error) {
	panes, err := t.ListPanes(window)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Role == RolePlatform {
			return p.ID, nil
		}
	}
	return "", nil
}
