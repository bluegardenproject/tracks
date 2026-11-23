package tracks

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestGithubRepo(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:acme/web.git":             "acme/web",
		"https://github.com/acme/web":             "acme/web",
		"https://user:token@github.com/acme/web/": "acme/web",
		"ssh://git@github.com/acme/web.git":       "acme/web",
		"git@gitlab.com:acme/web.git":             "",
		"/srv/git/web":                            "",
		"https://github.com/acme":                 "",
	} {
		if got := githubRepo(remote); got != want {
			t.Errorf("githubRepo(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestFastReview(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	web := repos[1].Path
	if filepath.Base(web) != "web" {
		t.Fatalf("the fixture's second repo is %s", web)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:Acme/Web.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", web}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gh := &fakeGitHub{title: "Fix the\n cache"}
	f.svc.GitHub = gh

	var p Problem
	if _, err := f.svc.FastReview(ctx, "not a link", nil); !errors.As(err, &p) {
		t.Errorf("a text that isn't a link: %v", err)
	}
	if _, err := f.svc.FastReview(ctx, "https://github.com/acme/other/pull/1", nil); !errors.As(err, &p) ||
		!strings.Contains(err.Error(), "acme/other") {
		t.Errorf("a PR of a repo that isn't on the Repositories tab: %v", err)
	}

	got, err := f.svc.FastReview(ctx, " https://github.com/acme/web/pull/42/files ", nil)
	if err != nil {
		t.Fatal(err)
	}
	tr := got.Track
	if tr.Kind != track.Review || tr.Title != "[Fast Review] Fix the cache" || tr.ReviewRef != "https://github.com/acme/web/pull/42" ||
		len(tr.Repos) != 1 || tr.Repos[0].Name != "web" || tr.Name != "review-pr-42" {
		t.Errorf("got %s %q %q %q %v", tr.Kind, tr.Name, tr.Title, tr.ReviewRef, tr.Repos)
	}

	gh.titleErr = errors.New("gh: not logged in")
	got, err = f.svc.FastReview(ctx, "https://github.com/acme/web/pull/43", nil)
	if err != nil || got.Track.Title != "[Fast Review] acme/web#43" {
		t.Errorf("without GitHub's title: %q, %v", got.Track.Title, err)
	}
}
