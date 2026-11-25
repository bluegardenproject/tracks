package tracks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/train"
)

// Trains are track id's trains, with their status: its reviewers,
// while their sessions are open.
func (s *Service) Trains(ctx context.Context, id string) ([]train.Train, error) {
	if _, err := s.Store.Track(ctx, id); errors.Is(err, store.ErrNotFound) {
		return nil, Problem("That track is gone.")
	} else if err != nil {
		return nil, err
	}
	if s.Reviewers == nil || s.HooksDir == "" {
		return nil, nil
	}
	// The panes and the runs are read together, as StartReview and
	// CloseReviewer change them together.
	s.reviewMu.Lock()
	panes, err := s.Reviewers.Reviewers()
	if err != nil {
		s.reviewMu.Unlock()
		return nil, err
	}
	runs, err := s.reviewerRuns(id, panes[id])
	s.reviewMu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.reviewerTrains(id, runs), nil
}

// reviewerRunState is what a reviewer's pane and run say of it.
type reviewerRunState struct {
	r       ReviewerState
	facts   train.ReviewerFacts
	sess    reviewSession
	sessErr error
	handle  string
	target  string
	// run names the review run, for telling a train waiting on the
	// user once a run.
	run string
}

// reviewerRuns are the runs of track id's reviewers whose panes are
// panes, by handle. A pane without a run in its folder, read before a
// review started or ended, has none, so it's no train. The caller holds
// s.reviewMu.
func (s *Service) reviewerRuns(id string, panes []ReviewerState) ([]reviewerRunState, error) {
	states, err := s.trainStates(id, train.Reviewer)
	if err != nil {
		return nil, err
	}
	var out []reviewerRunState
	for _, st := range states {
		i := slices.IndexFunc(panes, func(r ReviewerState) bool { return r.Handle == st.Handle && r.Pane != "" })
		if i < 0 || st.Run == "" {
			continue
		}
		r := panes[i]
		run := reviewerRunState{r: r, facts: train.ReviewerFacts{Pane: true, Exited: r.Exited}, handle: st.Handle,
			target: reviewTarget(st.Dir), run: st.Run}
		if b, err := os.ReadFile(filepath.Join(st.Dir, "report.md")); err == nil {
			run.facts.Report = string(b)
		}
		run.facts.Asking = asking(st.Dir)
		run.sess, run.sessErr = loadSession(st.Dir)
		out = append(out, run)
	}
	return out, nil
}

// reviewerTrains are track id's trains from its reviewers' runs.
func (s *Service) reviewerTrains(id string, runs []reviewerRunState) []train.Train {
	var out []train.Train
	for _, run := range runs {
		if tr, ok := s.reviewerTrain(id, run); ok {
			out = append(out, tr)
		}
	}
	return out
}

// reviewerTrain is the train of track id's reviewer on run.
func (s *Service) reviewerTrain(id string, run reviewerRunState) (train.Train, bool) {
	f := run.facts
	// Claude asks for trust as it starts, before any report: a report
	// that quotes the question isn't one.
	if f.Pane && !f.Exited && f.Report == "" && run.sessErr == nil && run.sess.Engine == agents.Claude.ID {
		if screen, err := s.Reviewers.CaptureReviewer(run.r.Pane); err == nil {
			f.AsksTrust = claude.AsksTrust(screen)
		}
	}
	status, outcome, ok := train.ReviewerStatus(f)
	if !ok {
		return train.Train{}, false
	}
	return train.Train{Track: id, Handle: run.handle, Agent: train.Reviewer, Engine: engineName(run.sess),
		Target: run.target, Status: status, Outcome: outcome, Held: s.heldFrom(id, run.handle)}, true
}

// EndTrain ends track id's train handle and closes its pane, as its End
// train button does. It returns the track's name.
func (s *Service) EndTrain(ctx context.Context, id, handle string) (string, error) {
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return "", Problem("That track is gone.")
	} else if err != nil {
		return "", err
	}
	trains, err := s.Trains(ctx, id)
	if err != nil {
		return t.Name, err
	}
	handles := make([]string, len(trains))
	for i, tr := range trains {
		handles[i] = tr.Handle
	}
	h, err := train.Resolve(handle, handles)
	if err != nil {
		return t.Name, Problem(fmt.Sprintf("%s: %v. tracks train list lists them.", t.Name, err))
	}
	// Reviewers are its only trains.
	return s.CloseReviewer(ctx, id, h)
}

// dropReview removes track id's reviews: their runs, reports and
// briefs, once their reviewers have gone with the track's window or the
// tmux server. Each says on the relay that its reviewer left, and why.
func (s *Service) dropReview(ctx context.Context, id, why string) error {
	if s.HooksDir == "" {
		return nil
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	states, err := s.trainStates(id, train.Reviewer)
	if err != nil {
		return err
	}
	for _, st := range states {
		if err := s.leftNotice(ctx, id, st.Dir, why); err != nil {
			return err
		}
		if err := os.RemoveAll(st.Dir); err != nil {
			return err
		}
	}
	return nil
}

// engineName is the agent and model a reviewer session runs on, for
// people; "" when it isn't known.
func engineName(sess reviewSession) string {
	e, ok := agents.ByID(sess.Engine)
	if !ok {
		return ""
	}
	if sess.Model != "" {
		return e.Name + " (" + sess.Model + ")"
	}
	return e.Name
}
