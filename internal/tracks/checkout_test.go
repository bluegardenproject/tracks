package tracks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/track"
)

// gitIn runs git in dir for a test.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.email=t@example.com", "-c", "user.name=T"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// checkouts are a repo, the track's worktree of it, a second worktree
// of it the agent added, and a clone of another repo.
func checkouts(t *testing.T) (repo, worktree, second, stranger string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := resolvedPath(t.TempDir())
	repo, worktree, second, stranger = filepath.Join(root, "repo"), filepath.Join(root, "wt"), filepath.Join(root, "fix"), filepath.Join(root, "other")
	for _, d := range []string{repo, stranger} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		gitIn(t, d, "init", "-q")
		gitIn(t, d, "commit", "-q", "--allow-empty", "-m", "base")
	}
	gitIn(t, repo, "worktree", "add", "-q", "-b", "track", worktree)
	gitIn(t, worktree, "worktree", "add", "-q", "-b", "fix", second)
	if err := os.Mkdir(filepath.Join(second, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	return repo, worktree, second, stranger
}

func TestCheckoutOf(t *testing.T) {
	repo, worktree, second, stranger := checkouts(t)
	tr := track.Track{Repos: []track.Repo{{Name: "api", Worktree: worktree, Base: "main"}}}
	// A worktree the agent added under the track's.
	nested := filepath.Join(worktree, ".worktrees", "fix")
	gitIn(t, worktree, "worktree", "add", "-q", "-b", "nested", nested)
	// A folder whose .git only claims to be a worktree of the repo.
	forged := t.TempDir()
	if err := os.WriteFile(filepath.Join(forged, ".git"), []byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "fix")), 0o600); err != nil {
		t.Fatal(err)
	}
	// One with a git folder of its own beside it, pointing at the repo.
	fake := t.TempDir()
	for name, text := range map[string]string{".git": "gitdir: .fake", ".fake/gitdir": filepath.Join(fake, ".git"),
		".fake/commondir": filepath.Join(repo, ".git"), ".fake/HEAD": "ref: refs/heads/main"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(fake, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fake, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// One git wrote with relative pointers.
	relative := filepath.Join(filepath.Dir(repo), "rel")
	gitIn(t, repo, "-c", "worktree.useRelativePaths=true", "worktree", "add", "-q", "-b", "rel", relative)
	if b, _ := os.ReadFile(filepath.Join(relative, ".git")); filepath.IsAbs(strings.TrimSpace(strings.TrimPrefix(string(b), "gitdir:"))) {
		t.Log("this git writes absolute pointers: the relative case is covered by the others")
	}
	for _, c := range []struct {
		dir   string
		other bool
		top   string
	}{
		{fake, true, ""},
		{relative, true, relative},
		{worktree, false, worktree},
		{worktree, true, worktree},
		{nested, false, ""},
		{nested, true, nested},
		{forged, true, ""},
		{second, false, ""},
		{filepath.Join(second, "sub"), true, second},
		{second, true, second},
		{repo, true, repo},
		{stranger, true, ""},
		{t.TempDir(), true, ""},
		{"relative", true, ""},
	} {
		r, top, own, ok := checkoutOf(tr, c.dir, c.other)
		if top != c.top || ok != (c.top != "") || (ok && r.Name != "api") || own != (top == worktree) {
			t.Errorf("checkoutOf(%s, other %v) = %q, %q, own %v, %v; want %q", c.dir, c.other, r.Name, top, own, ok, c.top)
		}
	}
}

// --dir reviews another checkout of a track's repo beside the track's
// own reviewer, and nothing that isn't one.
func TestReviewAnotherCheckout(t *testing.T) {
	ctx := context.Background()
	f, panes, _ := trainsFixture(t, "cursor")
	_, worktree, second, stranger := checkouts(t)
	if err := f.store.DeleteTrack(ctx, "r"); err != nil {
		t.Fatal(err)
	}
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	api := repos[0]
	api.CodeOwner = "@org/api"
	if _, err := f.store.UpdateRepo(ctx, api); err != nil {
		t.Fatal(err)
	}
	// A Review track: its pull request is its own worktree's.
	if err := f.store.AddTrack(ctx, track.Track{ID: "r", Kind: track.Review, Name: "r", Engine: "claude",
		ReviewRef: "https://github.com/o/r/pull/7", Repos: []track.Repo{{RepoID: api.ID, Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.svc.GitHub = &fakeGitHub{head: "aaaaaaaaaa"}
	packet := review.Packet{Head: "bbbbbbbbbb", Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: packet}); err == nil {
		t.Error("the track's own worktree, behind its pull request, was reviewed")
	}
	own, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree,
		Packet: review.Packet{Head: "aaaaaaaaaa", Diffs: packet.Diffs}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: second, Packet: packet}); err == nil {
		t.Error("another checkout was reviewed without --dir")
	}
	for _, bad := range []string{stranger, t.TempDir()} {
		if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: bad, Packet: packet, Other: true}); err == nil ||
			!strings.Contains(err.Error(), "isn't a checkout") {
			t.Errorf("--dir %s, not a checkout of the track's repos: %v", bad, err)
		}
	}
	fix, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: filepath.Join(second, "sub"), Packet: packet, Other: true})
	if err != nil {
		t.Fatal(err)
	}
	if fix.Target != second || len(panes.closed) != 0 || panes.dirs[len(panes.dirs)-1] != second {
		t.Fatalf("started %+v in %v, closed %v; want a reviewer of %s beside the track's own", fix, panes.dirs, panes.closed, second)
	}
	if brief, _ := os.ReadFile(filepath.Join(f.svc.trainDir("r", fix.Handle), "brief.md")); strings.Contains(string(brief), "Fix the cache") {
		t.Error("another checkout's review got the track's pull request")
	}
	msgs := relayOf(t, f)
	if got := msgs[len(msgs)-1].Body; got != "@"+fix.Handle+" joined: Reviewer on Cursor (m1), reviewing the change in "+second+"." {
		t.Errorf("joined notice %q; want it naming the checkout", got)
	}
	again, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: second, Packet: packet, Other: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(panes.closed) != 1 || panes.closed[0] != "r "+fix.Handle {
		t.Errorf("closed %v; a new review of %s replaces only its reviewer", panes.closed, second)
	}
	trains, err := f.svc.Trains(ctx, "r")
	if err != nil || len(trains) != 2 {
		t.Fatalf("trains %+v, %v", trains, err)
	}
	for _, tr := range trains {
		if want := map[string]string{own.Handle: worktree, again.Handle: second}[tr.Handle]; tr.Target != want {
			t.Errorf("%s reviews %q, want %q", tr.Handle, tr.Target, want)
		}
	}
	if owner, err := f.svc.ReviewOwner(ctx, "r", second); err != nil || owner != "@org/api" {
		t.Errorf("the code owner of another checkout: %q, %v; want its repo's", owner, err)
	}
	if _, err := f.svc.ReviewOwner(ctx, "r", stranger); err == nil {
		t.Error("a checkout of another repo was given a code owner")
	}
}
