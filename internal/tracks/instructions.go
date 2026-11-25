package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

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
