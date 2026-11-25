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

// askKind is the kind of an instruction asking a main track's agent to
// ask the user about an event of kind, as the "ask me" policy says.
func askKind(kind string) string { return "ask:" + kind }

// instructionKinds are the kinds of instruction about events of kind.
func instructionKinds(kind string) []string { return []string{kind, askKind(kind)} }

// eventKinds are the events a policy is set for.
var eventKinds = []string{settings.EventConflicts, settings.EventCIFailed, settings.EventReviewThreads}

// githubEvent tells of e, on a PR of t, a Work track: open, or a side
// track that ended with its PR still open. A track that isn't a side
// track hears it itself, @github's event waking its agent. A side
// track's events go to its main track, which keeps track of the work,
// and the side track hears only what its main track's policy has Tracks
// tell it: an agent told a fact acts on it, so it would act before the
// main track decided (see docs/design/trains.md, "Side tracks and
// GitHub's events").
func (s *Service) githubEvent(ctx context.Context, t track.Track, e ghEvent) error {
	if !side.Is(t) {
		if !t.Open() {
			return nil
		}
		return s.postGitHub(ctx, t.ID, e.Body, relay.Event)
	}
	main, err := s.Store.Track(ctx, t.Parent)
	switch {
	case errors.Is(err, store.ErrNotFound) || err == nil && !main.Open():
		// No main track to decide: an open side track hears it, as any
		// track.
		if !t.Open() {
			return nil
		}
		return s.postGitHub(ctx, t.ID, e.Body, relay.Event)
	case err != nil:
		return err
	}
	name := gitHubText(cmp.Or(t.Title, t.Name))
	named := fmt.Sprintf("@%s, side track %s: %s", t.Side, name, e.Body)
	policy := s.githubPolicy(e.Kind)
	if policy == settings.PolicyAuto && !t.Open() {
		// No agent to tell: its main track decides.
		policy = settings.PolicyMain
	}
	switch policy {
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
		if err := s.instruct(ctx, main.ID, askKind(e.Kind), e.PR.URL, actions.AskMain(t.Side, name, e.Body)); err != nil {
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

// dropDone drops the waiting instructions about pr that its state, as
// the poll read it, has made moot: all of them once it's merged or
// closed, conflicts' once it merges cleanly, a CI failure's once its
// checks don't fail.
func (s *Service) dropDone(ctx context.Context, pr track.PR) error {
	var kinds []string
	switch h := pr.Health; {
	case pr.State.Settled():
		for _, k := range eventKinds {
			kinds = append(kinds, instructionKinds(k)...)
		}
	default:
		if h.Mergeable == track.MergeClean {
			kinds = append(kinds, instructionKinds(settings.EventConflicts)...)
		}
		if h.Checks != track.ChecksFailing {
			kinds = append(kinds, instructionKinds(settings.EventCIFailed)...)
		}
	}
	return s.Store.DropInstructions(ctx, pr.URL, kinds...)
}
