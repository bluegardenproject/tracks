package tracks

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// PollPRs asks GitHub about the tracks' pull requests, in one pass:
// the PRs from the branches of Work tracks that aren't closed, which
// finds those opened by hand, then every PR not yet merged or closed,
// whatever its track's status. ErrNoGH means nothing was asked.
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
				if _, err := s.Store.SavePR(ctx, t.ID, pr, pr.CheckedAt); err != nil {
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
	for _, p := range unsettled {
		if asked[[2]string{p.TrackID, p.URL}] {
			continue
		}
		state, err := s.GitHub.PR(ctx, p.URL)
		if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		} else if err != nil {
			errs = append(errs, err)
			continue
		}
		p.State, p.CheckedAt = state, s.now()
		if _, err := s.Store.SavePR(ctx, p.TrackID, p.PR, p.CheckedAt); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
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
