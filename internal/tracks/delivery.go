package tracks

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
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
	if p, ok := s.pointing[id]; ok && turn == hooks.TurnStart && !p.collected {
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
	delete(s.turns, id)
	delete(s.pointing, id)
	delete(s.pointed, id)
	delete(s.retryAt, id)
	_, held := s.held[id]
	delete(s.held, id)
	s.mu.Unlock()
	if held {
		// Collect messages goes from the Platform.
		s.Changes.Notify()
	}
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
	// collected says the user sent it: it's done once pasted, and
	// neither a turn's start nor the stall check touches it meanwhile.
	collected bool
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
	waiting, err := s.Store.WaitingInstructions(ctx)
	if err != nil {
		return err
	}
	instructions := map[string][]store.Instruction{}
	for _, in := range waiting {
		instructions[in.TrackID] = append(instructions[in.TrackID], in)
	}
	var errs []error
	for _, t := range open {
		window, ok := windows[t.ID]
		if s.isBusy(t.ID) {
			continue
		}
		if !ok || t.Exit != "" {
			// No agent to point (its window, if any, holds the shell it
			// exited to): nothing to collect either.
			s.setHeld(t.ID, nil)
			continue
		}
		// An instruction goes first, and alone in its tick: the agent's
		// turn it starts holds the pointer back. One moot meanwhile gives
		// way to the next.
		typed, held := false, false
		for _, in := range instructions[t.ID] {
			var err error
			typed, held, err = s.typeInstruction(ctx, t, window, in)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
				held = true
			}
			if typed || held {
				break
			}
		}
		if typed {
			continue
		}
		// One held back waits ahead of the pointer, which still stalls,
		// and still shows held for Collect messages.
		if err := s.deliver(ctx, t, window, !held); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		}
	}
	return errors.Join(errs...)
}

// deliver points t's agent, in window, to its new messages; with paste
// false, an instruction waiting ahead of it, it only notes what's held
// and lets a stalled pointer go.
func (s *Service) deliver(ctx context.Context, t track.Track, window string, paste bool) error {
	now := s.now()
	s.mu.Lock()
	s.deliveryMaps()
	p, waiting := s.pointing[t.ID]
	stalled := waiting && !p.collected && now.Sub(p.at) >= pointerStall
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

	due, err := s.dueMessages(ctx, t, pointed, now, pointerGrace)
	if err != nil || len(due) == 0 {
		s.setHeld(t.ID, nil)
		return err
	}
	// The turn and a dialog hold the pointer back without asking tmux
	// anything. Held, its messages offer Collect messages on the
	// Platform.
	if now.Before(retry) || t.Waiting || s.inTurn(t.ID) || s.blockers(t, window).Why() != "" {
		s.setHeld(t.ID, due)
		return nil
	}
	if !paste {
		s.setHeld(t.ID, due)
		return nil
	}
	return s.point(t.ID, window, due, pointed, false)
}

// deliveryMaps makes delivery's maps. The caller holds s.mu.
func (s *Service) deliveryMaps() {
	if s.pointing == nil {
		s.pointing, s.pointed, s.retryAt = map[string]pointing{}, map[string]int64{}, map[string]time.Time{}
	}
	if s.turns == nil {
		s.turns = map[string]bool{}
	}
	if s.held == nil {
		s.held = map[string]map[string]bool{}
	}
}

// dueMessages are t's unread messages a pointer goes for, a side
// track's on its main track's relay too: past pointed, not a notice,
// and at least grace old.
func (s *Service) dueMessages(ctx context.Context, t track.Track, pointed int64, now time.Time, grace time.Duration) ([]relay.Message, error) {
	unread, err := s.unread(ctx, t, relay.TrackHandle)
	if err != nil {
		return nil, err
	}
	var due []relay.Message
	for _, m := range unread {
		if m.Kind != relay.Notice && m.ID > pointed && now.Sub(m.At) >= grace {
			due = append(due, m)
		}
	}
	return due, nil
}

// setHeld notes whose messages wait while their pointer is held back,
// nil for none, and tells the change feed when that changed: the
// Platform offers Collect messages under them.
func (s *Service) setHeld(id string, due []relay.Message) {
	from := map[string]bool{}
	for _, m := range due {
		from[m.From] = true
	}
	s.mu.Lock()
	s.deliveryMaps()
	changed := !maps.Equal(s.held[id], from)
	if len(from) == 0 {
		delete(s.held, id)
	} else {
		s.held[id] = from
	}
	s.mu.Unlock()
	if changed {
		s.Changes.Notify()
	}
}

// heldFrom reports whether a message from handle waits for track id's
// pointer, held back.
func (s *Service) heldFrom(id, handle string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[id][handle]
}

// point pastes the pointer to due into track id's agent, in window, and
// keeps its state. collected says the user sent it with Collect
// messages, past what holds it back: a turn on, unknown, doesn't stop it,
// and it isn't waited for as a turn's start, which an agent in a turn
// gives only after it, so it's done once pasted and the turn stays as
// it was.
func (s *Service) point(id, window string, due []relay.Message, pointed int64, collected bool) error {
	now := s.now()
	// Recorded before the paste, so a turn's start or a new agent
	// arriving meanwhile changes it, rather than the paste's end undoing
	// them. The pointer starts the agent's turn: none is known until its
	// hooks say so.
	s.mu.Lock()
	// Again, under the lock: the checks before asked tmux, unlocked, and
	// a turn may have started, a pointer gone or the track turned busy
	// meanwhile.
	in, known := s.turns[id]
	_, waiting := s.pointing[id]
	if s.busy[id] || s.pointed[id] != pointed || waiting || !collected && (!known || in) {
		s.mu.Unlock()
		if collected {
			return Problem("The messages changed meanwhile: try again.")
		}
		return nil
	}
	s.pointerGen++
	gen := s.pointerGen
	s.pointing[id] = pointing{at: now, before: pointed, gen: gen, collected: collected}
	s.pointed[id] = due[len(due)-1].ID
	if !collected {
		delete(s.turns, id)
		// Delivered, it's held no more: the Platform drops Collect
		// messages as the pointer goes. A collected one's goes once
		// it's in.
		delete(s.held, id)
	}
	delete(s.retryAt, id)
	s.mu.Unlock()
	// The messages count as pointed to from here, as the Relay Station
	// shows.
	s.Changes.Notify()

	err := s.Windows.Paste(window, relay.Pointer(due))
	s.mu.Lock()
	p, ours := s.pointing[id]
	switch {
	case !ours || p.gen != gen:
	case err != nil:
		delete(s.pointing, id)
		s.pointed[id] = p.before
		// A collected pointer left the turn as it was; a delivered one
		// went to an idle agent.
		if !collected {
			s.turns[id] = false
		}
		s.mu.Unlock()
		s.Changes.Notify()
		return err
	case collected:
		// Held no more once it's in, not before: the Platform keeps the
		// button, and its notice, while the paste may still fail.
		delete(s.pointing, id)
		delete(s.held, id)
		defer s.Changes.Notify()
	default:
		// The stall's wait starts once the pointer is in.
		p.at = s.now()
		s.pointing[id] = p
	}
	s.mu.Unlock()
	return err
}

// CollectMessages sends track id's agent the pointer to the messages
// that wait for one, now, past what holds it back: the user asked for
// it, and vouches for the agent's input. A question on its screen still
// holds it, which the pointer would answer; a pane scrolled back leaves
// its scrollback first, so the Enter submits. It returns the track's
// name.
func (s *Service) CollectMessages(ctx context.Context, id string) (string, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return "", err
	}
	infos, err := s.Windows.List()
	if err != nil {
		return t.Name, err
	}
	window := ""
	for _, in := range infos {
		if in.Track == id && in.Exit == "" {
			window = in.Window
		}
	}
	switch {
	case window == "" || t.Exit != "":
		return t.Name, Problem(t.Name + "'s agent isn't running.")
	case s.isBusy(id):
		return t.Name, Problem(t.Name + " is busy.")
	case t.Waiting:
		return t.Name, Problem(t.Name + "'s agent asks you something: answer it first.")
	}
	s.mu.Lock()
	s.deliveryMaps()
	_, waiting := s.pointing[id]
	pointed := s.pointed[id]
	s.mu.Unlock()
	if waiting {
		return t.Name, Problem("A pointer to " + t.Name + "'s messages is on its way.")
	}
	due, err := s.dueMessages(ctx, t, pointed, s.now(), 0)
	if err != nil {
		return t.Name, err
	}
	if len(due) == 0 {
		return t.Name, Problem("No messages wait for " + t.Name + ".")
	}
	// A pane whose mode can't be told might be scrolled back, where the
	// Enter would only leave the scrollback.
	scrolled, err := s.Windows.Scrolled(window)
	if err != nil {
		return t.Name, err
	}
	if scrolled {
		if err := s.Windows.LeaveScroll(window); err != nil {
			return t.Name, err
		}
	}
	return t.Name, s.point(id, window, due, pointed, true)
}

// typeInstruction pastes in, a waiting instruction, into t's agent in
// window when it's free, by what holds a pointer back, and reports
// whether it did. It's waited for as a pointer is: checked again under
// the lock, and kept as pointing until the agent's turn starts, so no
// pointer goes meanwhile and one the agent didn't take stalls as a
// pointer's does. One about review threads marks their Signals button
// sent. held says the agent isn't free; neither typed nor held, in was
// moot.
func (s *Service) typeInstruction(ctx context.Context, t track.Track, window string, in store.Instruction) (typed, held bool, err error) {
	s.mu.Lock()
	retry := s.retryAt[t.ID]
	s.mu.Unlock()
	// After a stall, it waits as long as a pointer would.
	if s.now().Before(retry) || t.Waiting || s.inTurn(t.ID) || s.blockers(t, window).Why() != "" {
		return false, true, nil
	}
	// Read a while ago: dropped or replaced since, it's moot.
	if still, err := s.Store.InstructionWaiting(ctx, in.ID); err != nil || !still {
		return false, err != nil, err
	}
	s.mu.Lock()
	s.deliveryMaps()
	if in.Kind == settings.EventReviewThreads && s.comments[t.ID][in.PR].Sent {
		// The user sent them from Signals.
		s.mu.Unlock()
		return false, false, s.Store.DropInstructions(ctx, in.PR, in.Kind)
	}
	_, waiting := s.pointing[t.ID]
	inTurn, known := s.turns[t.ID]
	if s.busy[t.ID] || waiting || !known || inTurn || s.now().Before(s.retryAt[t.ID]) {
		s.mu.Unlock()
		return false, true, nil
	}
	delete(s.retryAt, t.ID)
	s.pointerGen++
	gen := s.pointerGen
	s.pointing[t.ID] = pointing{at: s.now(), before: s.pointed[t.ID], gen: gen}
	delete(s.turns, t.ID)
	s.mu.Unlock()

	err = s.Windows.Paste(window, in.Body)
	s.mu.Lock()
	p, ours := s.pointing[t.ID]
	ours = ours && p.gen == gen
	if err != nil {
		if ours {
			// It went to an idle agent, which stays so.
			delete(s.pointing, t.ID)
			s.turns[t.ID] = false
		}
		s.mu.Unlock()
		return false, true, err
	}
	if ours {
		// The stall's wait starts once it's in.
		p.at = s.now()
		s.pointing[t.ID] = p
	}
	sent := false
	if in.Kind == settings.EventReviewThreads && s.comments[t.ID] != nil {
		if c, ok := s.comments[t.ID][in.PR]; ok && !c.Sent {
			s.setSent(t.ID, in.PR, true)
			sent = true
		}
	}
	s.mu.Unlock()
	if sent {
		// Signals shows them sent.
		s.Changes.Notify()
	}
	// It's in: marked so even if the daemon stops meanwhile.
	return true, false, s.Store.InstructionTyped(context.WithoutCancel(ctx), in.ID, s.now())
}
