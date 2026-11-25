package tracks

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// instructionAsk is an instruction asking a main track's agent to ask
// the user, as the "ask me" policy says.
const instructionAsk = "ask"

// githubEvent tells of e, on a PR of t, an open Work track. A track
// that isn't a side track hears it itself, @github's event waking its
// agent. A side track's events go to its main track, which keeps track
// of the work, and the side track hears only what its main track's
// policy has Tracks tell it: an agent told a fact acts on it, so it
// would act before the main track decided (see docs/design/trains.md,
// "Side tracks and GitHub's events").
func (s *Service) githubEvent(ctx context.Context, t track.Track, e ghEvent) error {
	if !side.Is(t) {
		return s.postGitHub(ctx, t.ID, e.Body, relay.Event)
	}
	main, err := s.Store.Track(ctx, t.Parent)
	switch {
	case errors.Is(err, store.ErrNotFound) || err == nil && !main.Open():
		// No main track to decide: the side track hears it, as any track.
		return s.postGitHub(ctx, t.ID, e.Body, relay.Event)
	case err != nil:
		return err
	}
	name := cmp.Or(t.Title, t.Name)
	named := fmt.Sprintf("@%s, side track %s: %s", t.Side, name, e.Body)
	switch s.githubPolicy(e.Kind) {
	case settings.PolicyAuto:
		body, ok := sideInstruction(t, e)
		if !ok {
			break
		}
		if err := s.instruct(ctx, t.ID, e.Kind, e.PR.URL, body); err != nil {
			return err
		}
		return s.postGitHub(ctx, main.ID, named+" Tracks tells it to handle it, once its agent is free.", relay.Notice)
	case settings.PolicyAsk:
		if err := s.instruct(ctx, main.ID, instructionAsk, e.PR.URL, actions.AskMain(t.Side, name, e.Body)); err != nil {
			return err
		}
		return s.postGitHub(ctx, main.ID, named+" Tracks asks the user through this track, once its agent is free.", relay.Notice)
	}
	// The main track decides: the event wakes its agent.
	return s.postGitHub(ctx, main.ID, named, relay.Event)
}

// githubPolicy is the user's policy for events of kind; merged and
// closed are always the main track's to decide.
func (s *Service) githubPolicy(kind string) settings.Policy {
	var set settings.Settings
	if s.Settings != nil {
		// Unread settings are the defaults.
		set, _ = s.Settings()
	}
	return set.GitHub.Policy(kind)
}

// sideInstruction is what Tracks types into side track t's agent for
// e; false for an event it has no prompt for.
func sideInstruction(t track.Track, e ghEvent) (string, bool) {
	pr := fmt.Sprintf("%s#%d", e.PR.Repo, e.PR.Number)
	h := e.PR.Health
	switch e.Kind {
	case settings.EventConflicts:
		return actions.SideConflicts(pr, baseOf(e.PR)), true
	case settings.EventCIFailed:
		head := "its head"
		if len(h.Head) >= 7 {
			head = gitHubText(h.Head[:7])
		}
		return actions.SideCIFailed(pr, head, failing(h.Failing)), true
	case settings.EventReviewThreads:
		return actions.SideThreads(t.Engine, pr, e.PR.URL, e.Threads)
	}
	return "", false
}

// instruct queues body for track id's agent, typed once it's free.
func (s *Service) instruct(ctx context.Context, id, kind, pr, body string) error {
	return s.Store.AddInstruction(ctx, store.Instruction{TrackID: id, Kind: kind, PR: pr, Body: body, CreatedAt: s.now()})
}
