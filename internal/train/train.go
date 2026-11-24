// Package train is the trains a track hands tasks to: their handles,
// their agents and their status (see docs/design/trains.md). The
// reviewer is the first.
package train

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/track"
)

// Agent is the kind of agent a train runs.
type Agent string

// Reviewer reviews a track's changes.
const Reviewer Agent = "reviewer"

// ReadsRelay says whether a's train reads its relay messages itself.
// The reviewer runs no commands, so it doesn't: Tracks types questions
// to it (tracks review --ask).
func (a Agent) ReadsRelay() bool { return false }

// Prefix starts the handles of a's trains: its name, until Settings
// sets one.
func (a Agent) Prefix() string { return string(a) }

// Name is how a is shown.
func (a Agent) Name() string {
	if a == "" {
		return ""
	}
	return strings.ToUpper(string(a[:1])) + string(a[1:])
}

// ReviewerHandle is the handle of a review started before reviews had
// handles of their own; a new review's is its prefix and a number.
const ReviewerHandle = "reviewer"

var handlePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// CheckHandle says why h can't be a train's handle, nil when it can:
// a lower-case letter, then up to 31 lower-case letters, digits and
// hyphens.
func CheckHandle(h string) error {
	if !handlePattern.MatchString(h) {
		return fmt.Errorf("%q isn't a train's name: a lower-case letter, then up to 31 lower-case letters, digits and hyphens", h)
	}
	return nil
}

// Train is one of a track's trains.
type Train struct {
	Track, Handle string
	Agent         Agent
	// Engine is the agent CLI and model it runs on, for people.
	Engine string
	// Status is a Station status; Outcome a second badge, such as a
	// review's, or none.
	Status, Outcome track.Status
}

// A review's outcome, a second badge beside the reviewer's status.
var (
	Pass    = track.Status{ID: "pass", Label: "pass", Badge: track.BadgeInfo}
	Blocked = track.Status{ID: "blocked", Label: "blocked", Badge: track.BadgeDanger}
)

// ReviewerFacts are what a reviewer's status is read from.
type ReviewerFacts struct {
	// Pane says the reviewer's pane is open: it closes as the reviewer
	// ends, by itself or not.
	Pane bool
	// Report is its report, "" before it's in.
	Report string
	// AsksTrust says its screen asks whether to trust the worktree.
	AsksTrust bool
	// Asking says a question to it waits for its answer.
	Asking bool
}

// ReviewerStatus is the status and outcome of a reviewer with facts f;
// ok is false when there's no reviewer. Without its pane there's none:
// what's left of a run doesn't say whether it ended or never started.
func ReviewerStatus(f ReviewerFacts) (status, outcome track.Status, ok bool) {
	if !f.Pane {
		return track.Status{}, track.Status{}, false
	}
	outcome = OutcomeOf(f.Report)
	switch {
	case f.Asking:
		return track.Active, outcome, true
	case notRun(f.Report):
		return track.Error, outcome, true
	case f.Report != "":
		return track.Done, outcome, true
	case f.AsksTrust:
		return track.ActionRequired, outcome, true
	}
	return track.Active, outcome, true
}

// notRun says report is the stand-in of a review that gave none.
func notRun(report string) bool {
	lines := strings.Split(strings.TrimSpace(report), "\n")
	return strings.TrimSpace(lines[len(lines)-1]) == review.Outcome+" "+review.NotRun
}

// OutcomeOf is report's outcome, Pass or Blocked from its last outcome
// line, or none.
func OutcomeOf(report string) track.Status {
	lines := strings.Split(report, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		rest, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), review.Outcome)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rest)) {
		case Pass.ID:
			return Pass
		case Blocked.ID:
			return Blocked
		}
		return track.Status{}
	}
	return track.Status{}
}

// StatusOf is the train status or outcome whose ID is id: a Station
// status, or Pass or Blocked; none for an ID it doesn't know.
func StatusOf(id string) track.Status {
	for _, s := range append([]track.Status{Pass, Blocked}, track.Statuses...) {
		if s.ID == id {
			return s
		}
	}
	return track.Status{}
}
