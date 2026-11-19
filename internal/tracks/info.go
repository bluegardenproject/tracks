package tracks

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
)

// SyncInfo shows each open track's info on its window, for the footer's
// top row beside its buttons. Only changed info is written.
func (s *Service) SyncInfo(ctx context.Context) error {
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	windows, err := s.trackWindows()
	if err != nil {
		return err
	}
	var reviewers map[string]ReviewerState
	if s.Reviewers != nil {
		reviewers, _ = s.Reviewers.Reviewers()
	}
	shown := map[string]string{}
	var errs []error
	for _, t := range open {
		window, ok := windows[t.ID]
		if !ok {
			continue
		}
		// The reviewer's button follows its pane, which ends with the
		// reviewer or a tracks close.
		if reviewers != nil {
			s.showReviewer(t, window, reviewers[t.ID])
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
	for id := range s.reviewers {
		if !slices.ContainsFunc(open, func(t track.Track) bool { return t.ID == id }) {
			delete(s.reviewers, id)
		}
	}
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
