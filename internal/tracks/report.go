package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/side"
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
	switch e {
	case track.Created, track.Resumed, track.Ended, track.Interrupted, track.AgentExited, track.AgentFailed:
		// A new agent, or none: what Tracks heard of the last one's
		// turns no longer holds.
		s.forgetTurn(id)
	}
	next := t.State.Apply(e, s.now())
	if next == t.State {
		return nil
	}
	if err := s.Store.SetState(ctx, id, next); err != nil {
		return err
	}
	s.notify(statusNotice(id, t.Name, t.Status(), next.Status()))
	s.sideStatus(ctx, t, next.Status())
	if t.Open() != next.Open() {
		s.sideNotice(ctx, t, e)
	}
	if (e == track.Ended || e == track.Archived) && side.Is(t) {
		// Done whatever happens to the caller now: the track has ended.
		if err := s.sideEnded(context.WithoutCancel(ctx), t); err != nil {
			return err
		}
	}
	if on := next.Status().Attention; on != t.Status().Attention && next.Open() {
		return s.markWindow(id, on)
	}
	return nil
}

// sideStatus tells side track t's main track, waking its agent, when
// t's status turns to needing the user or to an error: what it can't
// tell itself.
func (s *Service) sideStatus(ctx context.Context, t track.Track, to track.Status) {
	if !side.Is(t) || t.Parent == "" || to == t.Status() {
		return
	}
	var body string
	switch to {
	case track.ActionRequired:
		body = side.NeedsUser(t)
	case track.Error:
		body = side.Failed(t)
	default:
		return
	}
	m := relay.Message{From: relay.TracksHandle, To: relay.TrackHandle, Kind: relay.Event, Body: body, At: s.now()}
	_, _ = s.Store.AddRelayMessage(context.WithoutCancel(ctx), t.Parent, m)
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
