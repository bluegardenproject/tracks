package tracks

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/train"
)

// A track's trains each keep a folder of their own, named by handle, so
// any kind can run as several at once: trains/<handle>/ holds the
// train's kind and run marker, and its handle once it joined the relay.
// What a kind keeps beyond that, such as a reviewer's brief and report,
// goes in the same folder.

// trainState is one of a track's trains as its folder holds it.
type trainState struct {
	Handle, Dir string
	Agent       train.Agent
	// Run names its run, "" before it's written.
	Run string
}

// trainsDir holds track id's trains' folders.
func (s *Service) trainsDir(id string) string { return filepath.Join(s.HooksDir, id, "trains") }

// trainDir is the folder of track id's train handle.
func (s *Service) trainDir(id, handle string) string { return filepath.Join(s.trainsDir(id), handle) }

// legacyReviewDir held track id's one review before trains had folders
// of their own.
func (s *Service) legacyReviewDir(id string) string { return filepath.Join(s.HooksDir, id, "review") }

// trainStates are track id's trains of agent, "" for any, by handle,
// from their folders. A review's folder from before trains had folders
// of their own is read where it is, as its reviewer's (see
// legacyReview). The caller holds s.reviewMu.
func (s *Service) trainStates(id string, agent train.Agent) ([]trainState, error) {
	entries, err := os.ReadDir(s.trainsDir(id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var out []trainState
	for _, e := range entries {
		if !e.IsDir() || train.CheckHandle(e.Name()) != nil {
			continue
		}
		dir := filepath.Join(s.trainsDir(id), e.Name())
		st := trainState{Handle: e.Name(), Dir: dir, Agent: train.Agent(readTrim(filepath.Join(dir, "agent"))),
			Run: readTrim(filepath.Join(dir, "run"))}
		if agent == "" || st.Agent == agent {
			out = append(out, st)
		}
	}
	if st, ok := s.legacyReview(id); ok && (agent == "" || agent == train.Reviewer) &&
		!slices.ContainsFunc(out, func(o trainState) bool { return o.Handle == st.Handle }) {
		out = append(out, st)
	}
	slices.SortFunc(out, func(a, b trainState) int { return strings.Compare(a.Handle, b.Handle) })
	return out, nil
}

// legacyReview is track id's review from before trains had folders of
// their own, in review/, when there's one. It's read where it is, not
// moved: a Cursor reviewer started before writes its report into that
// folder by its path, so it lives there until it ends.
func (s *Service) legacyReview(id string) (trainState, bool) {
	dir := s.legacyReviewDir(id)
	if _, err := os.Stat(dir); err != nil {
		return trainState{}, false
	}
	return trainState{Handle: reviewHandle(dir), Dir: dir, Agent: train.Reviewer, Run: readTrim(filepath.Join(dir, "run"))}, true
}

// trainByHandle is track id's train handle; ok is false for none. The
// caller holds s.reviewMu.
func (s *Service) trainByHandle(id, handle string) (trainState, bool, error) {
	states, err := s.trainStates(id, "")
	if err != nil {
		return trainState{}, false, err
	}
	i := slices.IndexFunc(states, func(st trainState) bool { return st.Handle == handle })
	if i < 0 {
		return trainState{}, false, nil
	}
	return states[i], true, nil
}

// trainByRun is track id's train on run; ok is false for none, such as
// one since ended. The caller holds s.reviewMu.
func (s *Service) trainByRun(id, run string) (trainState, bool, error) {
	states, err := s.trainStates(id, "")
	if err != nil || run == "" {
		return trainState{}, false, err
	}
	for _, st := range states {
		if st.Run == run {
			return st, true, nil
		}
	}
	return trainState{}, false, nil
}

// readTrim is the file at path, trimmed; "" when it can't be read.
func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
