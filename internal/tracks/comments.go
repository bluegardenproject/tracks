package tracks

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/track"
)

// CountComments asks GitHub how many review threads wait on each open
// track's agent, on its PRs not merged or closed, and shows the count
// on the track's window for the footer's button. A track whose count
// couldn't be read keeps the last one. ErrNoGH means nothing was asked.
func (s *Service) CountComments(ctx context.Context) error {
	if s.GitHub == nil {
		return nil
	}
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	windows, err := s.trackWindows()
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range open {
		window, ok := windows[t.ID]
		if !ok {
			continue
		}
		n, err := s.openThreads(ctx, t)
		if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
			continue
		}
		// Written every poll, so a send between the count and this write
		// can't leave the window showing an older state for long.
		s.mu.Lock()
		if s.comments == nil {
			s.comments = map[string]actions.Comments{}
		}
		now := s.comments[t.ID].Next(n)
		s.comments[t.ID] = now
		s.mu.Unlock()
		if err := s.Windows.Comments(window, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// openThreads adds up the open threads on t's unsettled PRs.
func (s *Service) openThreads(ctx context.Context, t track.Track) (int, error) {
	total := 0
	for _, pr := range t.PRs {
		if pr.State.Settled() {
			continue
		}
		n, err := s.GitHub.OpenThreads(ctx, pr)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// trackWindows are the open track windows by track ID.
func (s *Service) trackWindows() (map[string]string, error) {
	infos, err := s.Windows.List()
	if err != nil {
		return nil, err
	}
	windows := map[string]string{}
	for _, in := range infos {
		windows[in.Track] = in.Window
	}
	return windows, nil
}

// RunAction sends track id's agent the prompt of action, and returns
// the track's name. It refuses while the agent can't take a prompt or
// there's nothing to do: Problems say why.
func (s *Service) RunAction(ctx context.Context, id, action string) (string, error) {
	if !actions.Known(action) {
		return "", Problem("Tracks doesn't know the action " + action + ".")
	}
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return "", err
	}
	switch {
	case !t.Open():
		return t.Name, Problem(t.Name + " has ended.")
	case t.Exit != "":
		return t.Name, Problem(t.Name + "'s agent has exited.")
	case t.Waiting:
		return t.Name, Problem(t.Name + "'s agent is waiting on you: answer it first.")
	}
	prompt, ok := actions.Prompt(action, t.Engine)
	if !ok {
		return t.Name, Problem(t.Name + "'s engine has no prompt for this.")
	}
	windows, err := s.trackWindows()
	if err != nil {
		return t.Name, err
	}
	window, ok := windows[id]
	if !ok {
		return t.Name, Problem(t.Name + " has no open window.")
	}

	s.mu.Lock()
	c := s.comments[id]
	switch {
	case c.Open == 0:
		s.mu.Unlock()
		return t.Name, Problem(t.Name + " has no open comments.")
	case c.Sent:
		s.mu.Unlock()
		return t.Name, Problem(t.Name + "'s agent already has them.")
	}
	c.Sent = true
	s.comments[id] = c
	s.mu.Unlock()

	if err := s.Windows.Prompt(window, prompt); err != nil {
		s.mu.Lock()
		c.Sent = false
		s.comments[id] = c
		s.mu.Unlock()
		return t.Name, err
	}
	return t.Name, s.Windows.Comments(window, c)
}
