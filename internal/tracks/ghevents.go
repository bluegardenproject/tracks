package tracks

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

// maxFailing is how many failing checks an event names.
const maxFailing = 5

// prEvents are @github's events on pr, which was in state was with
// health before: it merged or closed, has conflicts with its base, or
// its checks failed on a commit they hadn't failed on. A PR first seen
// already merged or closed has none.
func prEvents(pr track.PR, was track.PRState, before track.Health) []string {
	name := prName(pr)
	base := gitHubText(pr.Health.Base)
	if base == "" {
		base = "its base"
	}
	unsettled := was == track.PROpen || was == track.PRDraft
	switch {
	case pr.State == track.PRMerged && unsettled:
		return []string{name + " was merged into " + base + "."}
	case pr.State == track.PRClosed && unsettled:
		return []string{name + " was closed without merging."}
	case pr.State.Settled():
		return nil
	}
	var events []string
	h := pr.Health
	if h.Mergeable == track.MergeConflicts && before.Mergeable != track.MergeConflicts {
		events = append(events, name+" has conflicts with "+base+".")
	}
	if h.Checks == track.ChecksFailing && (before.Checks != track.ChecksFailing || before.Head != h.Head) {
		events = append(events, "CI failed on "+name+onCommit(h.Head)+": "+failing(h.Failing)+".")
	}
	return events
}

// threadsEvent is @github's event on pr's open review threads, now n,
// before was, "" when none came in.
func threadsEvent(pr track.PR, was, n int) string {
	switch {
	case n <= was:
		return ""
	case n == 1:
		return "1 review thread on " + prName(pr) + " waits for an answer."
	case n-was == n:
		return fmt.Sprintf("%d review threads on %s wait for an answer.", n, prName(pr))
	}
	return fmt.Sprintf("%d review threads on %s wait for an answer, %d of them new.", n, prName(pr), n-was)
}

// postGitHub posts body to track id's relay from @github, which wakes
// its agent.
func (s *Service) postGitHub(ctx context.Context, id, body string) error {
	_, err := s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.GitHubHandle, To: relay.TrackHandle,
		Kind: relay.Event, Body: body, At: s.now()})
	return err
}

func prName(pr track.PR) string { return fmt.Sprintf("PR %s#%d", pr.Repo, pr.Number) }

func onCommit(head string) string {
	if len(head) < 7 {
		return ""
	}
	return " at " + gitHubText(head[:7])
}

// failing names the failing checks, the first maxFailing of them.
func failing(names []string) string {
	var clean []string
	for _, n := range names {
		if n = gitHubText(n); n != "" {
			clean = append(clean, n)
		}
	}
	switch {
	case len(clean) == 0:
		return "a check"
	case len(clean) > maxFailing:
		return strings.Join(clean[:maxFailing], ", ") + fmt.Sprintf(" and %d more", len(clean)-maxFailing)
	}
	return strings.Join(clean, ", ")
}

// maxGitHubText is how much of a name read on GitHub an event keeps.
const maxGitHubText = 60

// gitHubText is s, a name read on GitHub, on one line of printable
// characters and cut short: a repo's owners choose their checks' and
// branches' names.
func gitHubText(s string) string {
	s = escapes.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxGitHubText {
		s = string(r[:maxGitHubText-1]) + "…"
	}
	return s
}
