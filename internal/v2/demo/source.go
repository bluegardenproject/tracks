package demo

import (
	"context"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// Source adds the fake tracks' details, such as engine and pull
// request, to the tracks another Source reads from their windows.
type Source struct{ Windows source.Source }

// Tracks lists the tracks with their fake details.
func (s Source) Tracks(ctx context.Context) ([]source.Track, error) {
	tracks, err := s.Windows.Tracks(ctx)
	if err != nil {
		return nil, err
	}
	fake := map[string]Track{}
	for _, t := range Tracks() {
		fake[t.Name] = t
	}
	for i, t := range tracks {
		f, ok := fake[t.Name]
		if !ok {
			continue
		}
		t.Engine, t.Model, t.Session, t.Cost = f.Engine, f.Model, f.Session, f.Cost
		for j := range t.Repos {
			t.Repos[j].Branch = f.Branch
		}
		if f.PR != 0 {
			t.PR = &source.PR{Number: f.PR, State: f.PRState,
				URL: fmt.Sprintf("https://github.com/example/%s/pull/%d", f.Repo, f.PR)}
		}
		tracks[i] = t
	}
	return tracks, nil
}
