package tracks

import (
	"context"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/track"
)

// Interrupt reports every open track without a window interrupted, and
// returns their names. The daemon runs it as it starts: windows only go
// away while it's down with their tmux server, so these tracks closed
// with Tracks rather than by the user.
func (s *Service) Interrupt(ctx context.Context) ([]string, error) {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return nil, err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, in := range infos {
		live[in.Track] = true
	}
	var names []string
	for _, t := range open {
		if live[t.ID] || s.isBusy(t.ID) {
			continue
		}
		if err := s.Report(ctx, t.ID, track.Interrupted); err != nil {
			return names, err
		}
		names = append(names, t.Name)
	}
	return names, nil
}

// Interrupted are the tracks to offer for reopening, oldest first.
func (s *Service) Interrupted(ctx context.Context) ([]track.Track, error) {
	return s.Store.InterruptedTracks(ctx)
}

// Reopening is how Reopen did with one track: the window it resumed
// in, or why it couldn't.
type Reopening struct {
	ID, Name string
	Window   string `json:",omitempty"`
	Error    string `json:",omitempty"`
}

// Reopen resumes the interrupted tracks, oldest first, telling progress
// which one it's at and its slow steps. A track that can't be resumed
// stays ended, interrupted, and Reopen goes on with the next.
func (s *Service) Reopen(ctx context.Context, progress func(string)) ([]Reopening, error) {
	interrupted, err := s.Interrupted(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Reopening, 0, len(interrupted))
	for _, t := range interrupted {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		progress(fmt.Sprintf("Reopening %s…", t.Name))
		r := Reopening{ID: t.ID, Name: t.Name}
		if c, err := s.Resume(ctx, t.ID, false, progress); err != nil {
			r.Error = err.Error()
		} else {
			r.Name, r.Window = c.Track.Name, c.Window.ID
		}
		out = append(out, r)
	}
	return out, nil
}
