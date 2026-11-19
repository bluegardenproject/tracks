// Package review holds what a track's reviewer is given, whichever
// engine runs it: its brief, the diff's limit, and the marker that tells
// a process it runs inside a review. The reviewer itself runs in a
// hidden pane of the track (see docs/design/review.md).
package review

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// EnvMarker is set in the reviewer's pane. A `tracks review` that sees
// it refuses: a reviewer doesn't start reviewers.
const EnvMarker = "TRACKS_REVIEW"

// ErrAlreadyReviewing is returned when a review is asked for from
// inside a review.
var ErrAlreadyReviewing = errors.New("already inside a review")

// Outcome starts the line a report ends with; a reply that has it is
// the review's report, one without it an answer to a question.
const Outcome = "REVIEW OUTCOME:"

// maxDiffBytes is a sanity bound on the diff. It exists only so a
// pathological diff fails fast with a readable note instead of spending
// the model's context on something no review can be made of.
const maxDiffBytes = 1_000_000

// Truncate cuts a diff over the sanity bound, with a note for the
// reviewer. A diff already cut is returned as it is, so a caller can
// cut it before sending it on.
func Truncate(diff string) string {
	if len(diff) <= maxDiffBytes || strings.HasSuffix(diff, truncatedEnd) {
		return diff
	}
	return diff[:maxDiffBytes] + "\n\n[diff truncated at " + strconv.Itoa(maxDiffBytes) + " bytes of " +
		strconv.Itoa(len(diff)) + truncatedEnd
}

// truncatedEnd ends the note Truncate adds.
const truncatedEnd = "; review what is shown and say in your report that it was truncated]"

// Brief is the file a reviewer is pointed at: the reviewer instructions,
// what it may do in its pane, the track's candor level and the diff.
func Brief(instructions string, candor int, diff string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(instructions))
	b.WriteString("\n\n## In this session\n\n" +
		"You run in a read-only mode, in a pane of the track Tracks keeps for its reviewer. " +
		"Read files in the worktree for context; the diff is below, so skip any instruction above " +
		"that asks you to run git or another command. Never edit a file, commit or push.\n\n" +
		"Write your report as your reply, ending with the `" + Outcome + "` line. After it, answer " +
		"questions about your review and the code you reviewed, and nothing else: you take no new " +
		"tasks, don't plan or build, and point anything else back to the track's agent.\n\n")
	if candor > 0 {
		fmt.Fprintf(&b, "Candor level: %d/10\n\n", candor)
	}
	b.WriteString("## The diff\n\n")
	b.WriteString(Truncate(diff))
	b.WriteString("\n")
	return b.String()
}

// Prompt is a reviewer's first message: where its brief is.
func Prompt(briefPath string) string {
	return "You are this track's reviewer. Read your brief at " + briefPath +
		" and follow it: it holds your instructions and the diff to review."
}

// PromptGiven is the first message of a reviewer whose brief came with
// its instructions.
const PromptGiven = "You are this track's reviewer. Your instructions hold your brief and the diff to review: follow them."
