package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/train"
)

// sideTrack checks that req asks for a side track it may have: a Work
// track of the open main track req.Parent, which isn't a side track
// itself. It fills in the main track's repos when req names none, and
// returns the main track.
func (s *Service) sideTrack(ctx context.Context, req *Request) (track.Track, error) {
	if req.Parent == "" {
		return track.Track{}, Problem("Start side tracks from inside a track.")
	}
	main, err := s.Store.Track(ctx, req.Parent)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return track.Track{}, Problem("That track is gone.")
	case err != nil:
		return track.Track{}, err
	case side.Is(main):
		return track.Track{}, Problem(main.Name + " is a side track: a side track starts no side tracks of its own.")
	case !main.Open():
		return track.Track{}, Problem(main.Name + " has ended.")
	}
	if req.Kind != track.Work {
		return track.Track{}, Problem("A side track is a Work track.")
	}
	if len(req.Repos) == 0 {
		for _, r := range main.Repos {
			req.Repos = append(req.Repos, r.Name)
		}
		if len(req.Repos) == 0 {
			return track.Track{}, Problem(main.Name + " has no repos: name the side track's with --repo.")
		}
	}
	return main, nil
}

// sideHandle is a new handle on main track id's relay, one none of its
// trains and side tracks has.
func (s *Service) sideHandle(ctx context.Context, id string) (string, error) {
	sides, err := s.Store.SideTracks(ctx, id)
	if err != nil {
		return "", err
	}
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, t := range sides {
		taken[t.Side] = true
	}
	for _, tr := range trains {
		taken[tr.Handle] = true
	}
	return train.NewHandle(side.Prefix, func(h string) bool { return taken[h] })
}

// SideTracks are main track id's side tracks, oldest first.
func (s *Service) SideTracks(ctx context.Context, id string) ([]track.Track, error) {
	if _, err := s.relayTrack(ctx, id); err != nil {
		return nil, err
	}
	return s.Store.SideTracks(ctx, id)
}

// makingSide notes that a side track of main track id is being made,
// until done: End, Archive and Derail refuse it meanwhile, and it
// refuses while one of them holds id. Both happen under s.mu, so one of
// the two always sees the other.
func (s *Service) makingSide(id string) (done func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[id] {
		return nil, Problem("The main track is busy: try again.")
	}
	if s.makingSides == nil {
		s.makingSides = map[string]int{}
	}
	s.makingSides[id]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.makingSides[id]--; s.makingSides[id] <= 0 {
			delete(s.makingSides, id)
		}
	}, nil
}

// sidesSettled refuses, as a Problem naming them, to let main track t
// be doing what doing says while a side track of its is unsettled.
func (s *Service) sidesSettled(ctx context.Context, t track.Track, doing string) error {
	s.mu.Lock()
	making := s.makingSides[t.ID] > 0
	s.mu.Unlock()
	if making {
		return Problem(t.Name + " is starting a side track: try again once it's open.")
	}
	sides, err := s.Store.SideTracks(ctx, t.ID)
	if err != nil {
		return err
	}
	if why := side.Waits(t.Name, doing, side.Left(sides)); why != "" {
		return Problem(why)
	}
	return nil
}

// sideNotice tells side track t's main track's relay that t joined it,
// or left it as it ended or Tracks stopped. A notice that can't be posted changes nothing
// for t.
func (s *Service) sideNotice(ctx context.Context, t track.Track, e track.Event) {
	if !side.Is(t) {
		return
	}
	body := side.Joined(t)
	switch e {
	case track.Ended:
		body = side.LeftNotice(t, "it ended")
	case track.Interrupted:
		body = side.LeftNotice(t, "Tracks stopped")
	case track.Created, track.Resumed:
	default:
		return
	}
	_ = s.notice(context.WithoutCancel(ctx), t.Parent, body)
}
