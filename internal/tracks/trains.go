package tracks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/train"
)

// Trains are track id's trains, with their status: its reviewer, while
// its pane is open.
func (s *Service) Trains(ctx context.Context, id string) ([]train.Train, error) {
	if _, err := s.Store.Track(ctx, id); errors.Is(err, store.ErrNotFound) {
		return nil, Problem("That track is gone.")
	} else if err != nil {
		return nil, err
	}
	if s.Reviewers == nil || s.HooksDir == "" {
		return nil, nil
	}
	window, _ := s.windowOf(id)
	// The pane and the run are read together, as StartReview and
	// CloseReviewer change them together.
	s.reviewMu.Lock()
	r, err := s.Reviewers.Reviewer(id, window)
	if err != nil || r.Pane == "" {
		s.reviewMu.Unlock()
		return nil, err
	}
	dir := s.reviewDir(id)
	f := train.ReviewerFacts{Pane: r.Pane != ""}
	if b, err := os.ReadFile(filepath.Join(dir, "report.md")); err == nil {
		f.Report = string(b)
	}
	f.Asking = s.asking(id)
	sess, sessErr := loadSession(dir)
	handle := reviewHandle(dir)
	s.reviewMu.Unlock()
	// Claude asks for trust as it starts, before any report: a report
	// that quotes the question isn't one.
	if r.Pane != "" && f.Report == "" && sessErr == nil && sess.Engine == agents.Claude.ID {
		if screen, err := s.Reviewers.CaptureReviewer(r.Pane); err == nil {
			f.AsksTrust = claude.AsksTrust(screen)
		}
	}
	status, outcome, ok := train.ReviewerStatus(f)
	if !ok {
		return nil, nil
	}
	return []train.Train{{Track: id, Handle: handle, Agent: train.Reviewer,
		Engine: engineName(sess), Status: status, Outcome: outcome}}, nil
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
	if _, err := train.Resolve(handle, handles); err != nil {
		return t.Name, Problem(fmt.Sprintf("%s: %v. tracks train list lists them.", t.Name, err))
	}
	// The reviewer is its only train.
	return s.CloseReviewer(ctx, id)
}

// dropReview removes track id's review: its run, report and brief, once
// its reviewer has gone with the track's window or the tmux server.
// A review there says on the relay that its reviewer left, and why.
func (s *Service) dropReview(ctx context.Context, id, why string) error {
	if s.HooksDir == "" {
		return nil
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	dir := s.reviewDir(id)
	if err := s.leftNotice(ctx, id, dir, why); err != nil {
		return err
	}
	return os.RemoveAll(dir)
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
