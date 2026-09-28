package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// isolate keeps git away from the user's config and any repo the test
// runs inside.
func isolate(t *testing.T) {
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo makes a primary checkout of name on main, cloned from a bare
// origin that also has the branch feat/x and the pull request ref 7.
func repo(t *testing.T, root, name string) track.Repo {
	origin := filepath.Join(root, name+".git")
	seed := filepath.Join(root, name+"-seed")
	run(t, root, "init", "-q", "--bare", "-b", "main", origin)
	run(t, root, "init", "-q", "-b", "main", seed)
	run(t, seed, "commit", "-q", "--allow-empty", "-m", "base")
	run(t, seed, "push", "-q", origin, "main")
	run(t, seed, "commit", "-q", "--allow-empty", "-m", "feature")
	run(t, seed, "push", "-q", origin, "HEAD:feat/x")
	run(t, seed, "commit", "-q", "--allow-empty", "-m", "pull request")
	run(t, seed, "push", "-q", origin, "HEAD:refs/pull/7/head")
	path := filepath.Join(root, name)
	run(t, root, "clone", "-q", origin, path)
	return track.Repo{Name: name, Path: path, Base: "main"}
}

func subject(t *testing.T, dir string) string { return run(t, dir, "log", "-1", "--format=%s") }

func noProgress(string) {}

func TestAddWork(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	api := repo(t, root, "api")
	run(t, api.Path, "branch", "tracks/abc123")
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}

	tr := track.Track{ID: "20260928-101500-abc123", Kind: track.Work, Repos: []track.Repo{api}}
	repos, err := w.Add(context.Background(), tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	got := repos[0]
	if want := filepath.Join(w.Root, tr.ID, "api"); got.Worktree != want {
		t.Errorf("worktree = %s, want %s", got.Worktree, want)
	}
	if got.Branch != "tracks/abc123-2" {
		t.Errorf("branch = %s, want the -2 suffix", got.Branch)
	}
	if b := run(t, got.Worktree, "branch", "--show-current"); b != got.Branch {
		t.Errorf("worktree is on %s", b)
	}
	if s := subject(t, got.Worktree); s != "base" {
		t.Errorf("worktree starts at %q, want origin/main", s)
	}
	if b := run(t, api.Path, "branch", "--show-current"); b != "main" {
		t.Errorf("the primary checkout moved to %s", b)
	}
}

func TestAddReview(t *testing.T) {
	isolate(t)
	for _, c := range []struct{ ref, label, subject string }{
		{"feat/x", "feat/x", "feature"},
		{"https://github.com/acme/api/pull/7/files", "pr/7", "pull request"},
	} {
		root := t.TempDir()
		w := &Worktrees{Root: filepath.Join(root, "worktrees")}
		tr := track.Track{ID: "20260928-101500-def456", Kind: track.Review, ReviewRef: c.ref, Repos: []track.Repo{repo(t, root, "api")}}
		repos, err := w.Add(context.Background(), tr, noProgress)
		if err != nil {
			t.Fatal(err)
		}
		if repos[0].Branch != c.label {
			t.Errorf("%s: branch = %s, want %s", c.ref, repos[0].Branch, c.label)
		}
		if s := subject(t, repos[0].Worktree); s != c.subject {
			t.Errorf("%s: checked out %q, want %q", c.ref, s, c.subject)
		}
	}
}

func TestAddRollsBack(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	api, web := repo(t, root, "api"), repo(t, root, "web")
	web.Base = "missing"
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	tr := track.Track{ID: "20260928-101500-abc123", Kind: track.Work, Repos: []track.Repo{api, web}}

	var steps []string
	if _, err := w.Add(context.Background(), tr, func(s string) { steps = append(steps, s) }); err == nil {
		t.Fatal("Add succeeded with a missing base")
	}
	if len(steps) < 3 {
		t.Errorf("progress = %q, want the fetch and worktree steps", steps)
	}
	if _, err := os.Stat(filepath.Join(w.Root, tr.ID)); !os.IsNotExist(err) {
		t.Errorf("the track's folder is still there: %v", err)
	}
	if out := run(t, api.Path, "branch", "--list", "tracks/*"); out != "" {
		t.Errorf("the new branch is still there: %s", out)
	}
	if out := run(t, api.Path, "worktree", "list"); strings.Count(out, "\n") != 0 {
		t.Errorf("a worktree is still registered:\n%s", out)
	}
}

func TestNoWorktrees(t *testing.T) {
	w := &Worktrees{Root: t.TempDir()}
	repos := []track.Repo{{Name: "api", Path: "/nowhere"}}
	got, err := w.Add(context.Background(), track.Track{ID: "x", Kind: track.Ask, Repos: repos}, noProgress)
	if err != nil || len(got) != 1 || got[0].Worktree != "" {
		t.Fatalf("Add for ask = %v, %v", got, err)
	}
}

func TestParseReview(t *testing.T) {
	for ref, want := range map[string]Review{
		"https://github.com/acme/api/pull/42":             {"pull/42/head", "pr/42"},
		" https://github.com/acme/api/pull/42#discussion": {"pull/42/head", "pr/42"},
		"feat/x": {"feat/x", "feat/x"},
	} {
		if got, err := ParseReview(ref); err != nil || got != want {
			t.Errorf("ParseReview(%q) = %v, %v", ref, got, err)
		}
	}
	for _, ref := range []string{"", "https://gitlab.com/acme/api/-/merge_requests/1", "github.com/acme/api"} {
		if _, err := ParseReview(ref); err == nil {
			t.Errorf("ParseReview(%q) accepted", ref)
		}
	}
}
