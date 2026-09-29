package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
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
"pr view https://github.com/a/b/pull/1 --json state,isDraft") echo '{"state":"OPEN","isDraft":true}' ;;
"pr view https://github.com/a/b/pull/2 --json state,isDraft") echo '{"state":"MERGED","isDraft":false}' ;;
*) echo "unknown command" >&2; exit 1 ;;
esac`)
	for url, want := range map[string]track.PRState{"https://github.com/a/b/pull/1": track.PRDraft, "https://github.com/a/b/pull/2": track.PRMerged} {
		if got, err := gh.PR(ctx, url); err != nil || got != want {
			t.Errorf("PR(%s) = %s, %v; want %s", url, got, err, want)
		}
	}

	dir := t.TempDir()
	gh = fakeGH(t, `[ "$*" = "pr list --head feat/x --state all --json url,state,isDraft" ] &&
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
