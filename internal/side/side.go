// Package side is what makes a track a side track: a Work track its
// main track started, on the main track's relay under a handle of its
// own (see docs/design/trains.md, Phase 2). It decides; the tracks
// package acts.
package side

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// Prefix starts a side track's handle: side-000123.
const Prefix = "side"

// Is reports whether t is a side track: one with a handle on the relay
// of a main track that's still there.
func Is(t track.Track) bool { return t.Side != "" && t.Parent != "" }

// Settled reports whether side track t leaves its main track free to
// end: it ended, and its pull requests are merged or closed, or it
// opened none.
func Settled(t track.Track) bool {
	if t.Open() {
		return false
	}
	for _, pr := range t.PRs {
		if !pr.State.Settled() {
			return false
		}
	}
	return true
}

// Left are the side tracks among sides that aren't settled.
func Left(sides []track.Track) []track.Track {
	var out []track.Track
	for _, t := range sides {
		if !Settled(t) {
			out = append(out, t)
		}
	}
	return out
}

// Waits says why main track name can't be done (ended, archived,
// derailed) while the side tracks left run on or keep a pull request
// open; "" when none is left.
func Waits(name, doing string, left []track.Track) string {
	if len(left) == 0 {
		return ""
	}
	parts := make([]string, len(left))
	for i, t := range left {
		parts[i] = t.Name + " (" + why(t) + ")"
	}
	noun := "side track"
	if len(left) > 1 {
		noun = "side tracks"
	}
	return fmt.Sprintf("%s waits for its %s before %s: %s. End each, and merge or close its pull request.",
		name, noun, doing, strings.Join(parts, ", "))
}

// why says what keeps side track t unsettled.
func why(t track.Track) string {
	var open []string
	for _, pr := range t.PRs {
		if !pr.State.Settled() {
			open = append(open, fmt.Sprintf("#%d", pr.Number))
		}
	}
	prs := ""
	switch len(open) {
	case 0:
	case 1:
		prs = "PR " + open[0] + " open"
	default:
		prs = "PRs " + strings.Join(open, ", ") + " open"
	}
	switch {
	case t.Open() && prs != "":
		return "running, " + prs
	case t.Open():
		return "running"
	}
	return prs
}
