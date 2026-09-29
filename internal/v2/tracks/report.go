package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Report records that e happened to track id: the one way a track's
// state changes, for the daemon's own steps and the agents' hooks.
// An event that doesn't fit the track's state changes nothing.
func (s *Service) Report(ctx context.Context, id string, e track.Event) error {
	if !e.Valid() {
		return Problem("Tracks doesn't know the event " + string(e) + ".")
	}
	s.reporting.Lock()
	defer s.reporting.Unlock()
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Problem("That track is gone.")
	} else if err != nil {
		return err
	}
	next := t.State.Apply(e, s.now())
	if next == t.State {
		return nil
	}
	return s.Store.SetState(ctx, id, next)
}

// reopen records that ended track id's window is open again, under
// name.
func (s *Service) reopen(ctx context.Context, id, name string) error {
	if err := s.Store.Rename(ctx, id, name); err != nil {
		return err
	}
	return s.Report(ctx, id, track.Resumed)
}
