package tracks

import (
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// typingQuiet is how long after a key in a track's window the user
// counts as typing there.
const typingQuiet = 3 * time.Second

// SetTurn notes whether track id's agent is in a turn, as its hooks
// tell: a prompt starts one, the agent stopping ends it.
func (s *Service) SetTurn(id, turn string) {
	if turn != hooks.TurnStart && turn != hooks.TurnEnd {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turns == nil {
		s.turns = map[string]bool{}
	}
	s.turns[id] = turn == hooks.TurnStart
}

// forgetTurn forgets what track id's agent's hooks told of its turns,
// as a new agent starts in its window or the track ends: until its
// next turn ends, it counts as in one.
func (s *Service) forgetTurn(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.turns, id)
}

// inTurn reports whether track id's agent may be in a turn: one Tracks
// hasn't heard end, or not heard of at all since it started.
func (s *Service) inTurn(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	in, known := s.turns[id]
	return in || !known
}

// blockers are what holds a pointer back from t's agent in window now.
// A screen that can't be read holds nothing back by itself: not typing
// is the guard left.
func (s *Service) blockers(t track.Track, window string) relay.Blockers {
	b := relay.Blockers{InTurn: s.inTurn(t.ID), Asking: t.Waiting}
	if last, err := s.Windows.LastKey(window); err != nil || !last.IsZero() && s.now().Sub(last) < typingQuiet {
		b.Typing = true
	}
	if screen, err := s.Windows.StyledScreen(window); err == nil {
		empty, known := hooks.InputEmpty(t.Engine, screen)
		b.Draft = known && !empty
	}
	return b
}
