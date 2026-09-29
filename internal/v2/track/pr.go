package track

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// PR is one of a track's pull requests.
type PR struct {
	URL       string // https://github.com/<owner>/<name>/pull/<number>
	Repo      string // <owner>/<name>
	Number    int
	State     PRState
	CheckedAt time.Time // zero until the poll asked GitHub
}

// PRState is where a pull request is.
type PRState string

const (
	PROpen   PRState = "open"
	PRDraft  PRState = "draft"
	PRMerged PRState = "merged"
	PRClosed PRState = "closed"
)

// Settled reports whether s can't change on its own any more, so the
// poll stops asking.
func (s PRState) Settled() bool { return s == PRMerged || s == PRClosed }

// prURL is a GitHub pull request link, with or without a trailing
// /files, #anchor or query.
var prURL = regexp.MustCompile(`^https://github\.com/([\w.-]+)/([\w.-]+)/pull/(\d+)(?:[/?#]\S*)?$`)

// ParsePR reads a pull request link into an open PR, its URL in one
// form so the same PR is found again.
func ParsePR(url string) (PR, bool) {
	m := prURL.FindStringSubmatch(url)
	if m == nil {
		return PR{}, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return PR{}, false
	}
	repo := m[1] + "/" + m[2]
	return PR{URL: "https://github.com/" + repo + "/pull/" + m[3], Repo: repo, Number: n, State: PROpen}, true
}

// The PR statuses, from a track's PRs; PRStatus words the label for
// how many there are.
var (
	NoPRs     = Status{ID: "none"}
	PRsOpen   = Status{ID: "open", Label: "PR open", Token: "state.info.text", Priority: 1}
	PRsMerged = Status{ID: "merged", Label: "PR merged", Token: "text.accent", Priority: 2}
	PRsClosed = Status{ID: "closed", Label: "PR closed", Token: "text.muted", Priority: 3}
)

// PRStatuses are every PR status shown, by priority.
var PRStatuses = []Status{PRsOpen, PRsMerged, PRsClosed}

// PRStatus is the PR status prs give: open while one is (a draft
// counts), else merged when one was, else closed.
func PRStatus(prs []PR) Status {
	var open, merged int
	for _, p := range prs {
		switch p.State {
		case PROpen, PRDraft:
			open++
		case PRMerged:
			merged++
		}
	}
	s := NoPRs
	switch {
	case open > 1:
		s = PRsOpen
		s.Label = fmt.Sprintf("%d PRs open", open)
	case open == 1:
		s = PRsOpen
	case merged > 1:
		s = PRsMerged
		s.Label = "PRs merged"
	case merged == 1:
		s = PRsMerged
	case len(prs) > 1:
		s = PRsClosed
		s.Label = "PRs closed"
	case len(prs) == 1:
		s = PRsClosed
	}
	return s
}
