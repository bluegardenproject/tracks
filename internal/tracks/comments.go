package tracks

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/track"
)

// CountComments asks GitHub how many review threads wait on each open
// track's agent, per PR not merged or closed, and shows a button on the
// track's window for each PR with some. A track whose counts couldn't be
// read keeps the last ones. ErrNoGH means nothing was asked.
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
		counts, err := s.openThreads(ctx, t)
		if errors.Is(err, ErrNoGH) || ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
			continue
		}
		// Written every poll, so a send between the count and this write
		// can't leave the window showing an older state for long.
		s.mu.Lock()
		was := s.comments[t.ID]
		now := map[string]actions.Comments{}
		for url, n := range counts {
			now[url] = was[url].Next(n)
		}
		if s.comments == nil {
			s.comments = map[string]map[string]actions.Comments{}
		}
		s.comments[t.ID] = now
		buttons := renderButtons(t, now)
		s.mu.Unlock()
		if err := s.Windows.Buttons(window, buttons); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// openThreads counts the open threads on each of t's unsettled PRs, by
// URL.
func (s *Service) openThreads(ctx context.Context, t track.Track) (map[string]int, error) {
	counts := map[string]int{}
	for _, pr := range t.PRs {
		if pr.State.Settled() {
			continue
		}
		n, err := s.GitHub.OpenThreads(ctx, pr)
		if err != nil {
			return nil, err
		}
		counts[pr.URL] = n
	}
	return counts, nil
}

// renderButtons are the buttons of t's PRs with open comments, in the
// order t found them, as the footer draws them.
func renderButtons(t track.Track, comments map[string]actions.Comments) string {
	var buttons []actions.Button
	repos := map[string]bool{}
	for i, pr := range t.PRs {
		repos[pr.Repo] = true
		if c := comments[pr.URL]; c.Open > 0 {
			buttons = append(buttons, actions.Button{Index: i, Repo: pr.Repo, Number: pr.Number, Comments: c})
		}
	}
	return actions.Render(buttons, len(repos) > 1)
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

// RunAction sends track id's agent the prompt of the button named
// button: to work through the open comments on one of its PRs. It
// returns the track's name, and refuses while the agent can't take a
// prompt or there's nothing to do: Problems say why.
func (s *Service) RunAction(ctx context.Context, id, button string) (string, error) {
	i, ok := actions.ParseRange(button)
	if !ok {
		return "", Problem("Tracks doesn't know the button " + button + ".")
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
	case i >= len(t.PRs):
		return t.Name, Problem(t.Name + " has no such PR.")
	}
	pr := t.PRs[i]
	prompt, ok := actions.Prompt(t.Engine, pr.URL)
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
	c := s.comments[id][pr.URL]
	switch {
	case c.Open == 0:
		s.mu.Unlock()
		return t.Name, Problem(t.Name + " has no open comments on PR #" + strconv.Itoa(pr.Number) + ".")
	case c.Sent:
		s.mu.Unlock()
		return t.Name, Problem(t.Name + "'s agent already has PR #" + strconv.Itoa(pr.Number) + "'s comments.")
	}
	s.setSent(id, pr.URL, true)
	s.mu.Unlock()

	if err := s.Windows.Prompt(window, prompt); err != nil {
		s.mu.Lock()
		s.setSent(id, pr.URL, false)
		s.mu.Unlock()
		return t.Name, err
	}
	s.mu.Lock()
	buttons := renderButtons(t, s.comments[id])
	s.mu.Unlock()
	return t.Name, s.Windows.Buttons(window, buttons)
}

// setSent marks the comments on track id's PR at url sent, or not. The
// caller holds s.mu.
func (s *Service) setSent(id, url string, sent bool) {
	c := s.comments[id][url]
	c.Sent = sent
	s.comments[id][url] = c
}
