package tracks

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/track"
)

// PollPRs asks GitHub about the tracks' pull requests, in one pass:
// the PRs from the branches of Work tracks that aren't closed, which
// finds those opened by hand, then every PR not yet merged or closed,
// whatever its track's status. What changed on an open Work track's
// PR goes to its relay as @github's events. ErrNoGH means nothing was
// asked.
func (s *Service) PollPRs(ctx context.Context) error {
	if s.GitHub == nil {
		return nil
	}
	tracks, err := s.branchTracks(ctx)
	if err != nil {
		return err
	}
	var errs []error
	asked := map[[2]string]bool{}
	for _, t := range tracks {
		for _, r := range t.Repos {
			if r.Branch == "" {
				continue
			}
			prs, err := s.GitHub.BranchPRs(ctx, r.Path, r.Branch)
			if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
				return errors.Join(err, ctx.Err())
			} else if err != nil {
				errs = append(errs, fmt.Errorf("%s in %s: %w", t.Name, r.Name, err))
				continue
			}
			for _, pr := range prs {
				pr.CheckedAt = s.now()
				was, err := s.Store.SavePR(ctx, t.ID, pr, pr.CheckedAt)
				if err != nil {
					return err
				}
				s.notify(prNotice(t.ID, t.Name, pr, was))
				if err := s.prHealth(ctx, t.ID, pr, was, t.Open()); err != nil {
					return err
				}
				asked[[2]string{t.ID, pr.URL}] = true
			}
		}
	}

	unsettled, err := s.Store.UnsettledPRs(ctx)
	if err != nil {
		return err
	}
	// Read after the branches' calls to GitHub, which take a while: a
	// track opened meanwhile gets its events.
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	working := map[string]bool{}
	for _, t := range open {
		working[t.ID] = t.Kind == track.Work
	}
	for _, p := range unsettled {
		if asked[[2]string{p.TrackID, p.URL}] {
			continue
		}
		now, err := s.GitHub.PR(ctx, p.URL)
		if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		} else if err != nil {
			errs = append(errs, err)
			continue
		}
		p.State, p.Health, p.CheckedAt = now.State, now.Health, s.now()
		was, err := s.Store.SavePR(ctx, p.TrackID, p.PR, p.CheckedAt)
		if err != nil {
			return err
		}
		if err := s.notifyPR(ctx, p.TrackID, p.PR, was); err != nil {
			return err
		}
		if err := s.prHealth(ctx, p.TrackID, p.PR, was, working[p.TrackID]); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

// prHealth records pr's health, as track id's, which was in state was,
// and posts @github's events on what changed when post says the track
// is an open Work track, whose agent acts on them.
func (s *Service) prHealth(ctx context.Context, id string, pr track.PR, was track.PRState, post bool) error {
	before := pr.Health
	if !pr.State.Settled() {
		var err error
		if before, err = s.Store.SavePRHealth(ctx, id, pr.URL, pr.Health); err != nil {
			return err
		}
	}
	if !post {
		return nil
	}
	for _, e := range prEvents(pr, was, before) {
		if err := s.postGitHub(ctx, id, e); err != nil {
			return err
		}
	}
	return nil
}

// branchTracks are the Work tracks whose branches may get PRs: those
// not closed, among the open and the last ended ones.
func (s *Service) branchTracks(ctx context.Context) ([]track.Track, error) {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return nil, err
	}
	done, err := s.Store.EndedTracks(ctx, ended)
	if err != nil {
		return nil, err
	}
	var out []track.Track
	for _, t := range append(open, done...) {
		if t.Kind == track.Work && !t.Archived() {
			out = append(out, t)
		}
	}
	return out, nil
}
