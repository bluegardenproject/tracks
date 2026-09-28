package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Listed is an open track and its window.
type Listed struct {
	track.Track
	Window string
	Number int // the window's index
}

// List returns the open tracks whose windows exist, in window order.
func (s *Service) List(ctx context.Context) ([]Listed, error) {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]track.Track, len(open))
	for _, t := range open {
		byID[t.ID] = t
	}
	infos, err := s.Windows.List()
	if err != nil {
		return nil, err
	}
	var out []Listed
	for _, in := range infos {
		if t, ok := byID[in.Track]; ok {
			out = append(out, Listed{Track: t, Window: in.Window, Number: in.Number})
		}
	}
	return out, nil
}

// End closes id's window and records that it closed. Its worktrees
// stay.
func (s *Service) End(ctx context.Context, id string) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	for _, in := range infos {
		if in.Track == id {
			if err := s.Windows.Close(in.Window); err != nil {
				return err
			}
		}
	}
	return s.Store.CloseTrack(ctx, id, s.now())
}

// Sweep records the open tracks whose window is gone as closed. A
// track's row is saved after its window opens, so a track being
// created is never swept.
func (s *Service) Sweep(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil || len(open) == 0 {
		return err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, in := range infos {
		live[in.Track] = true
	}
	for _, t := range open {
		if !live[t.ID] {
			if err := s.Store.CloseTrack(ctx, t.ID, s.now()); err != nil {
				return err
			}
		}
	}
	return nil
}
