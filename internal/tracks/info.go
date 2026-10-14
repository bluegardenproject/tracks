package tracks

import (
	"cmp"
	"context"
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
)

// SyncInfo shows each open track's info on its window, for the footer's
// top row while the track has no buttons. Only changed info is written.
func (s *Service) SyncInfo(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	windows, err := s.trackWindows()
	if err != nil {
		return err
	}
	shown := map[string]string{}
	var errs []error
	for _, t := range open {
		window, ok := windows[t.ID]
		if !ok {
			continue
		}
		text := Info(t)
		s.mu.Lock()
		same := s.info[window] == text
		s.mu.Unlock()
		if !same {
			if err := s.Windows.Info(window, text); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		shown[window] = text
	}
	s.mu.Lock()
	s.info = shown
	s.mu.Unlock()
	return errors.Join(errs...)
}

// Info is how the footer describes t: its name, its status, and the
// engine with the model it was started on.
func Info(t track.Track) string {
	engine := t.Engine
	if e, ok := agents.ByID(t.Engine); ok {
		engine = e.Name
	}
	parts := []string{cmp.Or(t.Title, t.Name), t.Status().Label, strings.TrimSpace(engine + " " + t.Model)}
	return strings.Join(parts, " · ")
}
