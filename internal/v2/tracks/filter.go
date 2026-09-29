package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// filtered is how many tracks Station lists at most with a filter on.
const filtered = 500

// Station is what Station lists, and the filter it's under: with none
// on, List's tracks; else every track the filter picks, newest first.
func (s *Service) Station(ctx context.Context) ([]Listed, track.Filter, error) {
	f, err := s.Store.Filter(ctx)
	if err != nil {
		return nil, f, err
	}
	if !f.On() {
		listed, err := s.List(ctx)
		return listed, f, err
	}
	found, err := s.Store.FilteredTracks(ctx, f, s.now(), filtered)
	if err != nil {
		return nil, f, err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return nil, f, err
	}
	out := make([]Listed, len(found))
	for i, t := range found {
		out[i] = Listed{Track: t}
		for _, in := range infos {
			if in.Track == t.ID && t.Open() {
				out[i].Window, out[i].Number = in.Window, in.Number
			}
		}
	}
	return out, f, nil
}

// Filter is Station's filter; the zero Filter when none is on.
func (s *Service) Filter(ctx context.Context) (track.Filter, error) {
	return s.Store.Filter(ctx)
}

// SetFilter puts Station under f, until it's cleared with the zero
// Filter.
func (s *Service) SetFilter(ctx context.Context, f track.Filter) error {
	if err := f.Check(); err != nil {
		return Problem(err.Error())
	}
	return s.Store.SetFilter(ctx, f)
}
