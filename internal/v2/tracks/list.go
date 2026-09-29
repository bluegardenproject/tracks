package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Listed is a track in Station's list: an open one and its window, or
// an ended one, without.
type Listed struct {
	track.Track
	Window string
	Number int // the window's index
}

// List returns the open tracks whose windows exist, in window order,
// then the most recently ended ones.
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
	done, err := s.Store.EndedTracks(ctx, ended)
	if err != nil {
		return nil, err
	}
	for _, t := range done {
		out = append(out, Listed{Track: t})
	}
	return out, nil
}

// End closes id's window and records that it closed. Its worktrees
// stay.
func (s *Service) End(ctx context.Context, id string) error {
	_, release, err := s.hold(ctx, id)
	if err != nil {
		return err
	}
	defer release()
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

// Sweep keeps the records in step with the windows: an open track
// whose window is gone is recorded as ended, and an ended one whose
// window exists as open again, under the window's name. Busy tracks
// are left to their Resume, Clean or End, and a track being created
// has no row until its window is open.
func (s *Service) Sweep(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
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
	isOpen := map[string]bool{}
	for _, t := range open {
		isOpen[t.ID] = true
		if !live[t.ID] && !s.isBusy(t.ID) {
			if err := s.Store.CloseTrack(ctx, t.ID, s.now()); err != nil {
				return err
			}
		}
	}
	for _, in := range infos {
		if isOpen[in.Track] || s.isBusy(in.Track) {
			continue
		}
		t, err := s.Store.Track(ctx, in.Track)
		if errors.Is(err, store.ErrNotFound) {
			continue
		} else if err != nil {
			return err
		}
		if !t.Open() {
			if err := s.Store.ReopenTrack(ctx, t.ID, in.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
