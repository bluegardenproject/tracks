package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
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
	if err := s.Store.SetState(ctx, id, next); err != nil {
		return err
	}
	s.notify(statusNotice(id, t.Name, t.Status(), next.Status()))
	if on := next.Status().Attention; on != t.Status().Attention && next.Open() {
		return s.markWindow(id, on)
	}
	return nil
}

// markWindow marks track id's window as needing the user, or not.
func (s *Service) markWindow(id string, on bool) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	for _, in := range infos {
		if in.Track == id {
			return s.Windows.Attention(in.Window, on)
		}
	}
	return nil
}

// reopen records that ended track id's window is open again, under
// name.
func (s *Service) reopen(ctx context.Context, id, name string) error {
	if err := s.Store.Rename(ctx, id, name); err != nil {
		return err
	}
	return s.Report(ctx, id, track.Resumed)
}
