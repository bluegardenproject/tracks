package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

// fakeGH writes a gh that prints out for its arguments, or fails.
func fakeGH(t *testing.T, script string) GH {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return GH{Program: path}
}

// pollAnswerJSON is a GraphQL answer to a poll for branch feat/x of
// a/b and PRs 4 and 9: the branch has PR 3, merged; PR 4 conflicts and
// fails its checks, with two threads waiting; PR 9's repo wasn't found.
const pollAnswerJSON = `{"data":{"viewer":{"login":"me"},
"b0":{"pullRequests":{"nodes":[{"url":"https://github.com/a/b/pull/3","state":"MERGED","isDraft":false,
  "mergeable":"UNKNOWN","baseRefName":"main","headRefOid":"aaa","commits":{"nodes":[]},"reviewThreads":{"nodes":[]}}]}},
"p0":{"pullRequest":{"url":"https://github.com/a/b/pull/4","state":"OPEN","isDraft":true,"mergeable":"CONFLICTING",
  "baseRefName":"main","headRefOid":"0123456789abcdef","commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
    {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE"},
    {"__typename":"CheckRun","name":"test","status":"IN_PROGRESS","conclusion":null},
    {"__typename":"StatusContext","context":"ci/deploy","state":"ERROR"}]}}}}]},
  "reviewThreads":{"nodes":[
    {"isResolved":false,"comments":{"nodes":[{"author":{"login":"copilot-pull-request-reviewer"}}]}},
    {"isResolved":false,"comments":{"nodes":[{"author":{"login":"me"}}]}},
    {"isResolved":true,"comments":{"nodes":[{"author":{"login":"alice"}}]}},
    {"isResolved":false,"comments":{"nodes":[{"author":null}]}},
    {"isResolved":false,"comments":{"nodes":[]}}]}}},
"p1":null},
"errors":[{"type":"NOT_FOUND","path":["p1"],"message":"Could not resolve to a Repository with the name c/d."}]}`

func TestGHPoll(t *testing.T) {
	ctx := context.Background()
	// Like gh, it prints the answer and fails for the error in it.
	gh := fakeGH(t, `case "$*" in
"api graphql -f query=query {"*"b0: repository(owner: \"a\", name: \"b\") { pullRequests(headRefName: \"feat/x\""*"p0: repository(owner: \"a\", name: \"b\") { pullRequest(number: 4)"*"p1: repository(owner: \"c\", name: \"d\") { pullRequest(number: 9)"*)
  [ "$GH_PROMPT_DISABLED" = 1 ] || exit 3
  echo '`+pollAnswerJSON+`'
  echo "gh: Could not resolve to a Repository with the name 'c/d'." >&2; exit 1 ;;
*) echo "unknown command: $*" >&2; exit 1 ;;
esac`)
	four, _ := track.ParsePR("https://github.com/a/b/pull/4")
	nine, _ := track.ParsePR("https://github.com/c/d/pull/9")
	branch := BranchQuery{Repo: "a/b", Branch: "feat/x"}
	a, err := gh.Poll(ctx, Query{Branches: []BranchQuery{branch}, PRs: []track.PR{four, nine}})
	if err == nil || !strings.Contains(err.Error(), nine.URL) || !strings.Contains(err.Error(), "Could not resolve") {
		t.Errorf("Poll's error = %v; want PR 9 named, with gh's reason", err)
	}
	if got := a.Branches[branch]; len(got) != 1 || got[0].URL != "https://github.com/a/b/pull/3" || got[0].State != track.PRMerged {
		t.Errorf("the branch's PRs = %+v; want #3 merged", got)
	}
	want := PRFacts{PR: four, Threads: 2}
	want.State = track.PRDraft
	want.Health = track.Health{Base: "main", Head: "0123456789abcdef", Mergeable: track.MergeConflicts,
		Checks: track.ChecksFailing, Failing: []string{"lint", "ci/deploy"}}
	if got := a.PRs[four.URL]; !reflect.DeepEqual(got, want) {
		t.Errorf("PR 4 = %+v\nwant %+v: threads by others, a deleted user's included", got, want)
	}
	if _, ok := a.PRs[nine.URL]; ok {
		t.Error("PR 9, which GitHub didn't find, is in the answer")
	}

	gh = fakeGH(t, `echo "HTTP 401: Bad credentials" >&2; exit 1`)
	if _, err := gh.Poll(ctx, Query{PRs: []track.PR{four}}); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("a failing gh: %v, want its message", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (GH{}).Poll(ctx, Query{PRs: []track.PR{four}}); !errors.Is(err, ErrNoGH) {
		t.Errorf("without gh: %v, want ErrNoGH", err)
	}
}

func TestGHPollChunks(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("CALLS", calls)
	gh := fakeGH(t, `echo x >> "$CALLS"; echo '{"data":{"viewer":{"login":"me"}}}'`)
	var prs []track.PR
	for n := 1; n <= 2*pollChunk+1; n++ {
		pr, _ := track.ParsePR("https://github.com/a/b/pull/" + strconv.Itoa(n))
		prs = append(prs, pr)
	}
	_, err := gh.Poll(context.Background(), Query{PRs: prs})
	if err == nil {
		t.Error("an answer without the PRs reads as one")
	}
	if b, _ := os.ReadFile(calls); strings.Count(string(b), "x") != 3 {
		t.Errorf("%d calls for %d PRs, want 3 of at most %d", strings.Count(string(b), "x"), len(prs), pollChunk)
	}
}

func TestPollAnswerErrors(t *testing.T) {
	four, _ := track.ParsePR("https://github.com/a/b/pull/4")
	pr := `{"pullRequest":{"url":"https://github.com/a/b/pull/4","state":"OPEN","isDraft":false,"mergeable":"MERGEABLE",
"baseRefName":"main","headRefOid":"abc","commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]},"reviewThreads":{"nodes":[]}}}`
	for name, tc := range map[string]struct {
		out  string
		read bool
	}{
		"whole":              {`{"data":{"viewer":{"login":"me"},"p0":` + pr + `}}`, true},
		"a field's error":    {`{"data":{"viewer":{"login":"me"},"p0":` + pr + `},"errors":[{"path":["p0","pullRequest","commits"]}]}`, false},
		"another's error":    {`{"data":{"viewer":{"login":"me"},"p0":` + pr + `},"errors":[{"path":["p1"]}]}`, true},
		"no viewer":          {`{"data":{"viewer":null,"p0":` + pr + `},"errors":[{"path":["viewer"]}]}`, false},
		"not GraphQL at all": {`HTTP 502`, false},
	} {
		a := Answer{Branches: map[BranchQuery][]PRFacts{}, PRs: map[string]PRFacts{}}
		err := pollAnswer([]byte(tc.out), nil, []track.PR{four}, a)
		if _, read := a.PRs[four.URL]; read != tc.read || (err == nil) != tc.read {
			t.Errorf("%s: read %v, error %v; want read %v", name, read, err, tc.read)
		}
	}
}

func TestPollQueryReadsThreadsOnlyByLink(t *testing.T) {
	four, _ := track.ParsePR("https://github.com/a/b/pull/4")
	q := pollQuery([]BranchQuery{{Repo: "a/b", Branch: "x"}}, []track.PR{four})
	if !strings.Contains(q, `headRefName: "x", first: 10, orderBy: {field: CREATED_AT, direction: DESC}) { nodes { ...pr } }`) ||
		!strings.Contains(q, `pullRequest(number: 4) { ...prThreads }`) {
		t.Errorf("a branch's PRs read with threads, or a PR's without:\n%s", q)
	}
}

func TestPollQueryQuotes(t *testing.T) {
	q := pollQuery([]BranchQuery{{Repo: "a/b", Branch: `x") { evil }`}}, nil)
	if !strings.Contains(q, `headRefName: "x\") { evil }"`) {
		t.Errorf("a branch's name isn't quoted:\n%s", q)
	}
}

func TestGHRepoOf(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DIR", dir)
	gh := fakeGH(t, `[ "$*" = "repo view --json nameWithOwner" ] && [ "$(pwd -P)" = "$(cd "$DIR" && pwd -P)" ] &&
echo '{"nameWithOwner":"acme/api"}'`)
	if repo, err := gh.RepoOf(context.Background(), dir); err != nil || repo != "acme/api" {
		t.Errorf("RepoOf = %q, %v", repo, err)
	}
	gh = fakeGH(t, `echo '{}'`)
	if _, err := gh.RepoOf(context.Background(), dir); err == nil {
		t.Error("an answer without a repo reads as one")
	}
}

func TestRollup(t *testing.T) {
	run := func(status, conclusion string) ghCheck {
		return ghCheck{Name: "c", Status: status, Conclusion: conclusion}
	}
	for _, tc := range []struct {
		checks []ghCheck
		want   track.Checks
	}{
		{nil, track.ChecksNone},
		{[]ghCheck{run("COMPLETED", "SUCCESS"), run("COMPLETED", "SKIPPED")}, track.ChecksPassing},
		{[]ghCheck{run("COMPLETED", "SUCCESS"), run("QUEUED", "")}, track.ChecksPending},
		{[]ghCheck{{Context: "c", State: "PENDING"}}, track.ChecksPending},
		{[]ghCheck{run("COMPLETED", "CANCELLED")}, track.ChecksPassing},
		{[]ghCheck{run("QUEUED", ""), run("COMPLETED", "TIMED_OUT")}, track.ChecksFailing},
		{[]ghCheck{{Context: "c", State: "FAILURE"}}, track.ChecksFailing},
	} {
		if got, _ := rollup(tc.checks); got != tc.want {
			t.Errorf("rollup(%+v) = %q, want %q", tc.checks, got, tc.want)
		}
	}
}
