package tracks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// typingQuiet is how long after a key in a track's window the user
// counts as typing there.
const typingQuiet = 3 * time.Second

// SetTurn notes whether track id's agent is in a turn, as its hooks
// tell: a prompt starts one, the agent stopping ends it. A track that
// ended, or whose agent exited, has no turn to note.
func (s *Service) SetTurn(ctx context.Context, id, turn string) {
	if turn != hooks.TurnStart && turn != hooks.TurnEnd {
		return
	}
	// Under Report's lock, so it can't land between Report reading the
	// track and forgetting its turn.
	s.reporting.Lock()
	defer s.reporting.Unlock()
	if t, err := s.Store.Track(ctx, id); err != nil || !t.Open() || t.Exit != "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turns == nil {
		s.turns = map[string]bool{}
	}
	s.turns[id] = turn == hooks.TurnStart
	if turn == hooks.TurnStart {
		// A pointer waiting for a turn was taken, however short the turn.
		delete(s.pointing, id)
	}
}

// forgetTurn forgets what track id's agent's hooks told of its turns,
// and what it was pointed to, as a new agent starts in its window or
// the track ends: until its next turn ends, it counts as in one, and
// its unread messages are pointed to again.
func (s *Service) forgetTurn(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.turns, id)
	delete(s.pointing, id)
	delete(s.pointed, id)
	delete(s.retryAt, id)
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
	if scrolled, err := s.Windows.Scrolled(window); err != nil || scrolled {
		b.Scrolled = true
	}
	if screen, err := s.Windows.StyledScreen(window); err == nil {
		empty, known := hooks.InputEmpty(t.Engine, screen)
		b.Draft = known && !empty
	}
	return b
}

// Delivery's timing: a message waits pointerGrace before a pointer goes
// for it, so a tracks review that prints it marks it read first; an
// agent that doesn't start a turn within pointerStall didn't take the
// pointer, which goes again after pointerRetry.
const (
	pointerGrace = 3 * time.Second
	pointerStall = 5 * time.Second
	pointerRetry = time.Minute
)

// pointing is a pointer typed into a track's agent, waiting for the
// turn it starts: sent when, how far the track was pointed to before
// it, and which pointer it is, so a paste that ends late touches only
// its own.
type pointing struct {
	at     time.Time
	before int64
	gen    uint64
}

// Deliver types a pointer into each open track's agent for the relay
// messages it hasn't read, once they've waited pointerGrace and nothing
// holds it back. Notices wake no one. A pointer goes once per message;
// one the agent didn't take, by not starting a turn within
// pointerStall, goes again after pointerRetry.
func (s *Service) Deliver(ctx context.Context) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	// A window whose agent exited holds a shell, where a pointer would
	// run as a command: it's skipped by its own mark, before the store
	// hears of the exit.
	windows := map[string]string{}
	for _, in := range infos {
		if in.Exit == "" {
			windows[in.Track] = in.Window
		}
	}
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range open {
		window, ok := windows[t.ID]
		if !ok || t.Exit != "" || s.isBusy(t.ID) {
			continue
		}
		if err := s.deliver(ctx, t, window); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		}
	}
	return errors.Join(errs...)
}

// deliver points t's agent, in window, to its new messages.
func (s *Service) deliver(ctx context.Context, t track.Track, window string) error {
	now := s.now()
	s.mu.Lock()
	if s.pointing == nil {
		s.pointing, s.pointed, s.retryAt = map[string]pointing{}, map[string]int64{}, map[string]time.Time{}
	}
	if s.turns == nil {
		s.turns = map[string]bool{}
	}
	p, waiting := s.pointing[t.ID]
	stalled := waiting && now.Sub(p.at) >= pointerStall
	switch {
	case stalled:
		// It didn't: it's as idle as it was when the pointer went.
		delete(s.pointing, t.ID)
		s.pointed[t.ID] = p.before
		s.retryAt[t.ID] = now.Add(pointerRetry)
		s.turns[t.ID] = false
	case waiting:
		s.mu.Unlock()
		return nil
	}
	pointed, retry := s.pointed[t.ID], s.retryAt[t.ID]
	s.mu.Unlock()
	if stalled {
		// Its messages wait for a pointer again, as the Relay Station
		// shows.
		s.Changes.Notify()
	}
	// The turn and a dialog hold the pointer back without asking the
	// store or tmux anything.
	if now.Before(retry) || t.Waiting || s.inTurn(t.ID) {
		return nil
	}

	unread, err := s.Store.RelayUnread(ctx, t.ID, relay.TrackHandle)
	if err != nil {
		return err
	}
	var due []relay.Message
	for _, m := range unread {
		if m.Kind != relay.Notice && m.ID > pointed && now.Sub(m.At) >= pointerGrace {
			due = append(due, m)
		}
	}
	if len(due) == 0 || s.blockers(t, window).Why() != "" {
		return nil
	}
	// Recorded before the paste, so a turn's start or a new agent
	// arriving meanwhile changes it, rather than the paste's end undoing
	// them. The pointer starts the agent's turn: none is known until its
	// hooks say so.
	s.mu.Lock()
	// Again, under the lock: the checks above asked tmux, unlocked, and
	// a turn may have started, a pointer gone or the track turned busy
	// meanwhile.
	if in, known := s.turns[t.ID]; !known || in || s.busy[t.ID] || s.pointed[t.ID] != pointed {
		s.mu.Unlock()
		return nil
	}
	if _, waiting := s.pointing[t.ID]; waiting {
		s.mu.Unlock()
		return nil
	}
	s.pointerGen++
	gen := s.pointerGen
	s.pointing[t.ID] = pointing{at: now, before: pointed, gen: gen}
	s.pointed[t.ID] = due[len(due)-1].ID
	delete(s.turns, t.ID)
	s.mu.Unlock()
	// The messages count as pointed to from here, as the Relay Station
	// shows.
	s.Changes.Notify()

	err = s.Windows.Paste(window, relay.Pointer(due))
	s.mu.Lock()
	p, ours := s.pointing[t.ID]
	switch {
	case !ours || p.gen != gen:
	case err != nil:
		delete(s.pointing, t.ID)
		s.pointed[t.ID] = p.before
		s.turns[t.ID] = false
		s.mu.Unlock()
		s.Changes.Notify()
		return err
	default:
		// The stall's wait starts once the pointer is in.
		p.at = s.now()
		s.pointing[t.ID] = p
	}
	s.mu.Unlock()
	return err
}
