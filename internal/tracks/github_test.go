package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestGH(t *testing.T) {
	ctx := context.Background()
	gh := fakeGH(t, `case "$*" in
"pr view https://github.com/a/b/pull/1 --json `+prFields+`") echo '{"state":"OPEN","isDraft":true}' ;;
"pr view https://github.com/a/b/pull/2 --json `+prFields+`") echo '{"state":"MERGED","isDraft":false}' ;;
*) echo "unknown command" >&2; exit 1 ;;
esac`)
	for url, want := range map[string]track.PRState{"https://github.com/a/b/pull/1": track.PRDraft, "https://github.com/a/b/pull/2": track.PRMerged} {
		if got, err := gh.PR(ctx, url); err != nil || got.State != want || got.URL != url {
			t.Errorf("PR(%s) = %+v, %v; want %s", url, got, err, want)
		}
	}

	dir := t.TempDir()
	gh = fakeGH(t, `[ "$*" = "pr list --head feat/x --state all --json `+prFields+`" ] &&
[ "$GH_PROMPT_DISABLED" = 1 ] && [ "$(pwd -P)" = "$(cd "$DIR" && pwd -P)" ] &&
echo '[{"url":"https://github.com/a/b/pull/3","state":"CLOSED","isDraft":false},{"url":"nope","state":"OPEN"}]'`)
	t.Setenv("DIR", dir)
	prs, err := gh.BranchPRs(ctx, dir, "feat/x")
	if err != nil || len(prs) != 1 || prs[0].URL != "https://github.com/a/b/pull/3" || prs[0].State != track.PRClosed {
		t.Errorf("BranchPRs = %+v, %v; want #3 closed, run in the repo without prompts", prs, err)
	}

	gh = fakeGH(t, `echo "HTTP 401: Bad credentials" >&2; exit 1`)
	if _, err := gh.PR(ctx, "https://github.com/a/b/pull/1"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("a failing gh: %v, want its message", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (GH{}).PR(ctx, "https://github.com/a/b/pull/1"); !errors.Is(err, ErrNoGH) {
		t.Errorf("without gh: %v, want ErrNoGH", err)
	}
}

func TestGHHealth(t *testing.T) {
	gh := fakeGH(t, `echo '{"url":"https://github.com/a/b/pull/4","state":"OPEN","isDraft":false,
"baseRefName":"main","headRefOid":"0123456789abcdef","mergeable":"CONFLICTING","statusCheckRollup":[
{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE"},
{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS","conclusion":""},
{"__typename":"CheckRun","name":"old","status":"COMPLETED","conclusion":"CANCELLED"},
{"__typename":"StatusContext","context":"ci/deploy","state":"ERROR"}]}'`)
	pr, err := gh.PR(context.Background(), "https://github.com/a/b/pull/4")
	want := track.Health{Base: "main", Head: "0123456789abcdef", Mergeable: track.MergeConflicts,
		Checks: track.ChecksFailing, Failing: []string{"lint", "ci/deploy"}}
	if err != nil || !reflect.DeepEqual(pr.Health, want) {
		t.Errorf("PR = %+v, %v; want health %+v", pr, err, want)
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

func TestGHOpenThreads(t *testing.T) {
	answer := `{"data":{"viewer":{"login":"me"},"repository":{"pullRequest":{"reviewThreads":{"nodes":[
{"isResolved":false,"comments":{"nodes":[{"author":{"login":"copilot-pull-request-reviewer"}}]}},
{"isResolved":false,"comments":{"nodes":[{"author":{"login":"me"}}]}},
{"isResolved":true,"comments":{"nodes":[{"author":{"login":"alice"}}]}},
{"isResolved":false,"comments":{"nodes":[{"author":null}]}},
{"isResolved":false,"comments":{"nodes":[]}}
]}}}}}`
	gh := fakeGH(t, `case "$*" in
"api graphql -f query="*" -f owner=acme -f name=api -F number=7") echo '`+answer+`' ;;
*) echo "unknown command: $*" >&2; exit 1 ;;
esac`)
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/7")
	if n, err := gh.OpenThreads(context.Background(), pr); err != nil || n != 2 {
		t.Errorf("OpenThreads = %d, %v; want 2: the bot's, and the deleted user's", n, err)
	}
}
