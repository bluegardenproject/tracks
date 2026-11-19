// Package review holds what a track's reviewer is given, whichever
// engine runs it: its brief, the diff's limit, and the marker that tells
// a process it runs inside a review. The reviewer itself runs in a
// hidden pane of the track (see docs/design/review.md).
package review

import (
	"errors"
	"fmt"
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

// Brief is the file a reviewer is pointed at: the reviewer instructions,
// what it may do in its pane, the track's candor level and the packet.
func Brief(instructions string, candor int, p Packet) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(instructions))
	b.WriteString("\n\n## In this session\n\n" +
		"You run in a read-only mode, in a pane of the track Tracks keeps for its reviewer. " +
		"Read files in the worktree for context; the change is below, so skip any instruction above " +
		"that asks you to run git or another command. Never edit a file, commit or push.\n\n" +
		"Write your report as your reply, ending with the `" + Outcome + "` line. After it, answer " +
		"questions about your review and the code you reviewed, and nothing else: you take no new " +
		"tasks, don't plan or build, and point anything else back to the track's agent.\n\n")
	if candor > 0 {
		fmt.Fprintf(&b, "Candor level: %d/10\n\n", candor)
	}
	if p.Base != "" {
		fmt.Fprintf(&b, "The change is against `%s`.\n\n", p.Base)
	}
	if brief := strings.TrimSpace(p.Brief); brief != "" {
		b.WriteString("## Asked of this review\n\n" + brief + "\n\n")
	}
	if commits := strings.TrimSpace(p.Commits); commits != "" {
		b.WriteString("## The commits\n\nWhat the change is for, in its commits' words, newest first. " +
			"Check the change does what they say.\n\n" + commits + "\n\n")
	}
	b.WriteString("## Changed files\n\n")
	for _, f := range p.Files {
		if f.Binary {
			fmt.Fprintf(&b, "- %s (binary)\n", f.Path)
		} else {
			fmt.Fprintf(&b, "- %s +%d −%d\n", f.Path, f.Added, f.Removed)
		}
	}
	if p.MoreFiles > 0 {
		fmt.Fprintf(&b, "- and %d more files, over the list's limit\n", p.MoreFiles)
	}
	if len(p.Left) > 0 {
		b.WriteString("\nOver the diff's size limit, so their diffs aren't below: " + strings.Join(p.Left, ", ") +
			". Don't review them; name them on your report's Not run line.\n")
	}
	b.WriteString("\n## The diff\n\n")
	inUncommitted := false
	for _, d := range p.Diffs {
		if d.Uncommitted && !inUncommitted {
			b.WriteString("\n--- uncommitted changes (not yet in a commit) ---\n\n")
			inUncommitted = true
		}
		b.WriteString(d.Text)
	}
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
