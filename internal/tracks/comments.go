package tracks

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/track"
)

// CountComments asks GitHub how many review threads wait on each open
// track's agent, per PR not merged or closed; the Platform's Signals
// shows a button for each PR with some, and the change feed tells it
// when they changed. More of them on a Work track's PR is @github's
// event on its relay. A track whose counts couldn't be read keeps the
// last ones. ErrNoGH means nothing was asked.
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
		if err := s.threadsEvents(ctx, t, counts); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		}
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
		// Compared under the lock: RunAction marks a PR's comments sent
		// in this map.
		changed := !maps.Equal(was, now)
		s.mu.Unlock()
		if changed {
			s.Changes.Notify()
		}
		// The footer has no buttons of its own now: a window opened
		// before the PRs' buttons moved to Signals loses its old ones.
		if err := s.Windows.Buttons(window, ""); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// threadsEvents records the open threads counted on t's PRs, and posts
// @github's events for a Work track's PRs that have more than before.
// The first count is no event: the threads may have waited long.
func (s *Service) threadsEvents(ctx context.Context, t track.Track, counts map[string]int) error {
	for _, pr := range t.PRs {
		n, ok := counts[pr.URL]
		if !ok {
			continue
		}
		was, known, err := s.Store.SavePRThreads(ctx, t.ID, pr.URL, n)
		if err != nil {
			return err
		}
		if e := threadsEvent(pr, was, n); known && e != "" && t.Kind == track.Work {
			if err := s.postGitHub(ctx, t.ID, e); err != nil {
				return err
			}
		}
	}
	return nil
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

// PRSignal is one of a track's PRs with open comments, for Signals: the
// PR, what its button does, and whether that's done, the prompt sent.
type PRSignal struct {
	Index      int
	PR, Action string
	Sent       bool
}

// prSignals are t's PRs with open comments, as comments counted them.
func prSignals(t track.Track, comments map[string]actions.Comments) []PRSignal {
	repos := map[string]bool{}
	for _, pr := range t.PRs {
		repos[pr.Repo] = true
	}
	var out []PRSignal
	for i, pr := range t.PRs {
		if c := comments[pr.URL]; c.Open > 0 {
			b := actions.Button{Index: i, Repo: pr.Repo, Number: pr.Number, Comments: c}
			out = append(out, PRSignal{Index: i, PR: b.PR(len(repos) > 1), Action: b.Action(), Sent: c.Sent})
		}
	}
	return out
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
	// Signals shows it sent.
	s.Changes.Notify()
	return t.Name, nil
}

// setSent marks the comments on track id's PR at url sent, or not. The
// caller holds s.mu.
func (s *Service) setSent(id, url string, sent bool) {
	c := s.comments[id][url]
	c.Sent = sent
	s.comments[id][url] = c
}
