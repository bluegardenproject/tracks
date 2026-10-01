package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/track"
)

// Exit is an agent CheckExits found exited: its track's name and the
// code it exited with.
type Exit struct {
	Name, Code string
}

// CheckExits reports the open tracks whose agent exited, as its wrapper
// saved in the track's window, and returns those it reported: each
// once, since the window keeps the code until the agent restarts.
func (s *Service) CheckExits(ctx context.Context) ([]Exit, error) {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return nil, err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return nil, err
	}
	codes := map[string]string{}
	for _, in := range infos {
		codes[in.Track] = in.Exit
	}
	var exits []Exit
	for _, t := range open {
		code := codes[t.ID]
		if code == "" || s.isBusy(t.ID) {
			continue
		}
		e, exit := track.AgentFailed, track.ExitFailed
		if code == "0" {
			e, exit = track.AgentExited, track.ExitOK
		}
		if t.Exit == exit {
			continue
		}
		if err := s.Report(ctx, t.ID, e); err != nil {
			return exits, err
		}
		exits = append(exits, Exit{Name: t.Name, Code: code})
	}
	return exits, nil
}
