package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/hooks"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// closedAfter is how many checks in a row must find a waiting track's
// dialog gone before it counts as closed, so one still being drawn
// isn't taken for one that closed.
const closedAfter = 2

// CheckScreens looks at the agent panes of tracks waiting on a dialog,
// and reports those whose dialog closed without a hook saying so, such
// as one dismissed with Esc.
func (s *Service) CheckScreens(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	windows := map[string]string{}
	for _, in := range infos {
		windows[in.Track] = in.Window
	}
	gone := map[string]int{}
	for _, t := range open {
		window, ok := windows[t.ID]
		if !t.Waiting || !ok || s.isBusy(t.ID) {
			continue
		}
		screen, err := s.Windows.Screen(window)
		if err != nil {
			continue
		}
		if dialog, known := hooks.DialogOpen(t.Engine, screen); !known || dialog {
			continue
		}
		if gone[t.ID] = s.dialogGone(t.ID) + 1; gone[t.ID] >= closedAfter {
			if err := s.Report(ctx, t.ID, track.AgentWorking); err != nil {
				return err
			}
			delete(gone, t.ID)
		}
	}
	s.mu.Lock()
	s.gone = gone
	s.mu.Unlock()
	return nil
}

func (s *Service) dialogGone(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gone[id]
}
