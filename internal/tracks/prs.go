package tracks

import (
	"context"
	"errors"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// SeePRs records that track id's agent opened the pull requests at
// urls. A PR already known stays as it is, and a Review track's
// reviewed PR isn't one of its own.
func (s *Service) SeePRs(ctx context.Context, id string, urls []string) error {
	if len(urls) == 0 {
		return nil
	}
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Problem("That track is gone.")
	} else if err != nil {
		return err
	}
	for _, u := range urls {
		pr, ok := track.ParsePR(u)
		if !ok || reviewed(t, pr) {
			continue
		}
		added, err := s.Store.AddPR(ctx, id, pr, s.now())
		if err != nil {
			return err
		}
		if added {
			s.notify(prNotice(id, t.Name, pr, ""))
		}
	}
	return nil
}

// reviewed reports whether pr is the one Review track t reviews.
func reviewed(t track.Track, pr track.PR) bool {
	r, ok := track.ParsePR(t.ReviewRef)
	return t.Kind == track.Review && ok && r.URL == pr.URL
}
