package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestParsePRView(t *testing.T) {
	pr, head, err := parsePRView([]byte(`{"url":"https://github.com/o/r/pull/7","title":"Fix","body":"Why","headRefOid":"abc123",
	"statusCheckRollup":[
	 {"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/o/r/actions/runs/1/job/42"},
	 {"__typename":"CheckRun","name":"lint","status":"IN_PROGRESS","conclusion":""},
	 {"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED"},
	 {"__typename":"StatusContext","context":"ci/legacy","state":"SUCCESS","targetUrl":"https://ci.example/1"}]}`))
	if err != nil || head != "abc123" || pr.Title != "Fix" || pr.Body != "Why" {
		t.Fatalf("pr %+v, head %q, %v", pr, head, err)
	}
	want := []review.Check{
		{Name: "test", State: review.CheckFail, Link: "https://github.com/o/r/actions/runs/1/job/42"},
		{Name: "lint", State: review.CheckPending},
		{Name: "docs", State: review.CheckSkipped},
		{Name: "ci/legacy", State: review.CheckPass, Link: "https://ci.example/1"},
	}
	if !slices.Equal(pr.Checks, want) {
		t.Errorf("checks %+v", pr.Checks)
	}
	if repo, job := actionsJobOf(pr.Checks[0].Link); repo != "o/r" || job != "42" {
		t.Errorf("job %s of %s", job, repo)
	}
	if _, job := actionsJobOf("https://ci.example/1"); job != "" {
		t.Error("an Actions job's link wasn't told from another")
	}
}

func TestPlain(t *testing.T) {
	if got := plain("\x1b[31mFAIL\x1b[0m\tx_test.go\r\n\x1b]0;title\x07ok\x00\u009b2J\n"); got != "FAIL\tx_test.go\nok2J\n" {
		t.Errorf("plain = %q", got)
	}
	w := &tail{limit: 4}
	for _, s := range []string{"abc", "defgh", "ij"} {
		_, _ = w.Write([]byte(s))
	}
	if got := string(w.buf); !strings.HasSuffix(got, "ghij") || len(got) > 8 {
		t.Errorf("tail kept %q", got)
	}
}

func TestParseThreads(t *testing.T) {
	threads, more, err := parseThreads([]byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"totalCount":5,"nodes":[
	 {"isResolved":true,"path":"a.go","comments":{"nodes":[{"author":{"login":"x"},"body":"done"}]}},
	 {"isResolved":false,"isOutdated":true,"path":"b.go","line":12,"comments":{"totalCount":3,"nodes":[{"author":{"login":"ana"},"body":"Why?\u001b[2J"},{"author":null,"body":"Because."}]}}]}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Path != "b.go" || threads[0].Line != 12 || !threads[0].Outdated ||
		len(threads[0].Comments) != 2 || threads[0].Comments[1].Author != "ghost" || threads[0].Comments[0].Body != "Why?" ||
		threads[0].EarlierComments != 1 || more != 3 {
		t.Errorf("threads %+v, %d more; want the unresolved one, with its comments, plain, and what wasn't fetched counted", threads, more)
	}
}

func TestAddPR(t *testing.T) {
	ctx := context.Background()
	gh := &fakeGitHub{head: "aaaaaaaaaa", reviewNotes: []string{"the log of test (gone)"}}
	s := &Service{GitHub: gh}
	review_ := track.Track{Kind: track.Review, ReviewRef: "https://github.com/o/r/pull/7"}

	p, err := s.addPR(ctx, review_, review.Packet{Head: "aaaaaaaaaa"}, false)
	if err != nil || p.PR == nil || p.PR.Title != "Fix the cache" || !slices.Equal(p.Notes, gh.reviewNotes) {
		t.Errorf("packet %+v, %v; want the PR and the notes on what's missing", p, err)
	}
	if _, err := s.addPR(ctx, review_, review.Packet{Head: "bbbbbbbbbb"}, false); err == nil ||
		!strings.Contains(err.Error(), "moved on") || !strings.Contains(err.Error(), "pull/7/head") {
		t.Errorf("a worktree behind the PR: %v; want it refused, with how to catch up", err)
	}
	p, err = s.addPR(ctx, review_, review.Packet{Head: "bbbbbbbbbb"}, true)
	if err != nil || p.PR == nil || !strings.Contains(strings.Join(p.Notes, " "), "older head") {
		t.Errorf("--stale: %+v, %v; want the review, with a note", p.Notes, err)
	}
	for _, other := range []track.Track{{Kind: track.Work, ReviewRef: review_.ReviewRef}, {Kind: track.Review, ReviewRef: "a-branch"}} {
		if p, _ := s.addPR(ctx, other, review.Packet{}, false); p.PR != nil {
			t.Errorf("%+v got a PR", other)
		}
	}
	gh.reviewErr = errors.New("gh isn't logged in")
	if p, err := s.addPR(ctx, review_, review.Packet{}, false); err != nil || p.PR != nil || len(p.Notes) != 1 {
		t.Errorf("GitHub out of reach: %+v, %v; want the review to go on, with a note", p, err)
	}
}
