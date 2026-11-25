package tracks

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
)

// maxFailing is how many failing checks an event names.
const maxFailing = 5

// ghEvent is one of @github's events on PR: Kind is a settings.Event*
// for those a policy is set for, eventMerged or eventClosed; Body says
// it. Threads counts a review threads event's open threads.
type ghEvent struct {
	Kind, Body string
	PR         track.PR
	Threads    int
}

// The events no policy is set for: the main track decides.
const (
	eventMerged = "merged"
	eventClosed = "closed"
)

// prEvents are @github's events on pr, which was in state was with
// health before: it merged or closed, has conflicts with its base, or
// its checks failed on a commit they hadn't failed on. A PR first seen
// already merged or closed has none.
func prEvents(pr track.PR, was track.PRState, before track.Health) []ghEvent {
	name := prName(pr)
	base := baseOf(pr)
	unsettled := was == track.PROpen || was == track.PRDraft
	switch {
	case pr.State == track.PRMerged && unsettled:
		return []ghEvent{{Kind: eventMerged, PR: pr, Body: name + " was merged into " + base + "."}}
	case pr.State == track.PRClosed && unsettled:
		return []ghEvent{{Kind: eventClosed, PR: pr, Body: name + " was closed without merging."}}
	case pr.State.Settled():
		return nil
	}
	var events []ghEvent
	h := pr.Health
	if h.Mergeable == track.MergeConflicts && before.Mergeable != track.MergeConflicts {
		events = append(events, ghEvent{Kind: settings.EventConflicts, PR: pr, Body: name + " has conflicts with " + base + "."})
	}
	if h.Checks == track.ChecksFailing && (before.Checks != track.ChecksFailing || before.Head != h.Head) {
		events = append(events, ghEvent{Kind: settings.EventCIFailed, PR: pr,
			Body: "CI failed on " + name + onCommit(h.Head) + ": " + failing(h.Failing) + "."})
	}
	return events
}

// threadsEvent is @github's event on pr's open review threads, now n,
// before was; false when none came in.
func threadsEvent(pr track.PR, was, n int) (ghEvent, bool) {
	e := ghEvent{Kind: settings.EventReviewThreads, PR: pr, Threads: n}
	switch {
	case n <= was:
		return ghEvent{}, false
	case n == 1:
		e.Body = "1 review thread on " + prName(pr) + " waits for an answer."
	case n-was == n:
		e.Body = fmt.Sprintf("%d review threads on %s wait for an answer.", n, prName(pr))
	default:
		e.Body = fmt.Sprintf("%d review threads on %s wait for an answer, %d of them new.", n, prName(pr), n-was)
	}
	return e, true
}

// baseOf is the branch pr merges into, as an event names it.
func baseOf(pr track.PR) string {
	if base := gitHubText(pr.Health.Base); base != "" {
		return base
	}
	return "its base"
}

// postGitHub posts body to track id's relay from @github, an event
// that wakes its agent, or a notice, which doesn't.
func (s *Service) postGitHub(ctx context.Context, id, body string, kind relay.Kind) error {
	_, err := s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.GitHubHandle, To: relay.TrackHandle,
		Kind: kind, Body: body, At: s.now()})
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

// safeRef matches a branch name Tracks puts into a command it has an
// agent run: letters, digits, '.', '_', '/' and '-', starting with a
// letter, digit or '_', with no "..", at most 200. Git and GitHub allow more
// (backticks, $(), ;), which a command would run.
var safeRef = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]{0,199}$`)

// gitHubRef is ref, a branch's name read on GitHub, for a git or gh
// command an agent runs; false for one that isn't safe there.
func gitHubRef(ref string) (string, bool) {
	if !safeRef.MatchString(ref) || strings.Contains(ref, "..") {
		return "", false
	}
	return ref, true
}

// shortSHA matches a commit's first 7 hex digits.
var shortSHA = regexp.MustCompile(`^[0-9a-f]{7}$`)
