package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/train"
)

// showBadge writes badges into the header of the train in session,
// unless they're there already. A failed write is tried again on the
// next tick.
func (s *Service) showBadge(session, badges string) {
	s.mu.Lock()
	same := s.badges[session] == badges
	s.mu.Unlock()
	if same || s.Reviewers.TrainBadge(session, badges) != nil {
		return
	}
	s.mu.Lock()
	if s.badges == nil {
		s.badges = map[string]string{}
	}
	s.badges[session] = badges
	s.mu.Unlock()
}

// tellWaiting tells the user that tr, a train of track id on run, waits
// on them: once a run, so a question left unanswered isn't told on
// every tick.
func (s *Service) tellWaiting(ctx context.Context, id string, tr train.Train, run string) {
	s.mu.Lock()
	told := s.trainTold[id] == run
	if !told {
		if s.trainTold == nil {
			s.trainTold = map[string]string{}
		}
		s.trainTold[id] = run
	}
	s.mu.Unlock()
	if told || s.Notify == nil {
		return
	}
	if t, err := s.Store.Track(ctx, id); err == nil {
		s.notify(trainWaitingNotice(id, t.Name, tr.Handle))
	}
}
