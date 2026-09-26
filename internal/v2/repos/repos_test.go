package repos

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/store"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Env = append(gitEnv(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// clone makes a repo with an origin whose default branch is trunk, and
// returns the clone's path, symlinks resolved.
func clone(t *testing.T, name string) string {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	origin := filepath.Join(root, "origin")
	git(t, root, "init", "-q", "-b", "trunk", origin)
	git(t, origin, "commit", "-q", "--allow-empty", "-m", "first")
	git(t, origin, "branch", "develop")
	git(t, root, "clone", "-q", origin, name)
	return filepath.Join(root, name)
}

func service(t *testing.T, uses ...Use) Service {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tracks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return Service{Store: s, Git: Exec{}, Uses: func(context.Context) ([]Use, error) { return uses, nil }}
}

func field(err error) string {
	var f *FieldError
	if errors.As(err, &f) {
		return f.Field
	}
	return ""
}

func TestSuggestReadsTheCheckout(t *testing.T) {
	dir := clone(t, "Shop Web")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, link, filepath.Join(dir, "src")} {
		got, err := service(t).Suggest(context.Background(), path)
		if err != nil {
			t.Fatalf("Suggest(%s): %v", path, err)
		}
		if got.Path != dir || got.Name != "Shop Web" || got.BaseBranch != "trunk" {
			t.Errorf("Suggest(%s) = %+v; want the top folder, Shop Web, trunk", path, got)
		}
	}
}

func TestGitIgnoresTheCallersRepo(t *testing.T) {
	decoy, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, decoy, "init", "-q")
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	dir := clone(t, "shop")
	if got, err := service(t).Suggest(context.Background(), dir); err != nil || got.Path != dir {
		t.Errorf("Suggest(%s) = %+v, %v; want the checkout at dir", dir, got, err)
	}
	if err := exec.Command("git", "-C", decoy, "--git-dir", filepath.Join(decoy, ".git"), "rev-parse", "--verify", "--quiet", "HEAD").Run(); err == nil {
		t.Error("git commands reached the repo named by GIT_DIR")
	}
}

func TestAddChecksEveryField(t *testing.T) {
	ctx := context.Background()
	dir := clone(t, "shop")
	worktree := filepath.Join(filepath.Dir(dir), "wt")
	git(t, dir, "worktree", "add", "-q", worktree)
	plain := t.TempDir()

	s := service(t)
	for _, c := range []struct {
		repo  store.Repo
		field string
	}{
		{store.Repo{Name: "  ", Path: dir, BaseBranch: "trunk"}, "name"},
		{store.Repo{Name: "shop\tweb", Path: dir, BaseBranch: "trunk"}, "name"},
		{store.Repo{Name: strings.Repeat("ü", 65), Path: dir, BaseBranch: "trunk"}, "name"},
		{store.Repo{Name: "shop", Path: "relative/path", BaseBranch: "trunk"}, "path"},
		{store.Repo{Name: "shop", Path: filepath.Join(dir, "missing"), BaseBranch: "trunk"}, "path"},
		{store.Repo{Name: "shop", Path: plain, BaseBranch: "trunk"}, "path"},
		{store.Repo{Name: "shop", Path: worktree, BaseBranch: "trunk"}, "path"},
		{store.Repo{Name: "shop", Path: dir, BaseBranch: "nope"}, "base"},
		{store.Repo{Name: "shop", Path: dir, BaseBranch: "--all"}, "base"},
		{store.Repo{Name: "shop", Path: dir}, "base"}, // main, which this repo lacks
	} {
		if _, err := s.Add(ctx, c.repo); field(err) != c.field {
			t.Errorf("Add(%+v): %v; want a problem with %s", c.repo, err, c.field)
		}
	}

	added, err := s.Add(ctx, store.Repo{Name: " Shop Web ", Path: dir + "/", BaseBranch: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	if added.Name != "Shop Web" || added.Path != dir {
		t.Errorf("added %+v; want the name trimmed and the path cleaned", added)
	}
	if _, err := s.Add(ctx, store.Repo{Name: "other", Path: dir, BaseBranch: "trunk"}); field(err) != "path" {
		t.Errorf("adding the same checkout twice: %v; want a path problem", err)
	}
	if _, err := s.Add(ctx, store.Repo{Name: "shop web", Path: clone(t, "other"), BaseBranch: "trunk"}); field(err) != "name" {
		t.Errorf("adding a second repo called shop web: %v; want a name problem", err)
	}
}

func TestRunningTracksPinTheRepo(t *testing.T) {
	ctx := context.Background()
	s := service(t, Use{Repo: "Shop", Track: "fix-login"})
	dir := clone(t, "shop")
	r, err := s.Add(ctx, store.Repo{Name: "shop", Path: dir, BaseBranch: "trunk"})
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "remote", "set-url", "origin", "git@github.com:acme/shop.git")
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || !slices.Equal(list[0].Tracks, []string{"fix-login"}) ||
		list[0].Remote != "https://github.com/acme/shop" {
		t.Fatalf("List = %+v, %v; want shop with its web address and fix-login", list, err)
	}

	r.BaseBranch, r.DraftPRs = "develop", true
	if _, err := s.Update(ctx, r); err != nil {
		t.Errorf("changing base and drafts while tracks run: %v", err)
	}
	r.Name = "shop2"
	if _, err := s.Update(ctx, r); !errors.Is(err, ErrInUse) {
		t.Errorf("renaming while tracks run: %v; want ErrInUse", err)
	}
	if err := s.Delete(ctx, r.ID); !errors.Is(err, ErrInUse) {
		t.Errorf("deleting while tracks run: %v; want ErrInUse", err)
	}

	idle := Service{Store: s.Store, Git: Exec{}}
	if _, err := idle.Update(ctx, r); err != nil {
		t.Errorf("renaming without tracks: %v", err)
	}
	if err := idle.Delete(ctx, r.ID); err != nil {
		t.Errorf("deleting without tracks: %v", err)
	}
}

func TestWebURL(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:acme/shop.git":                 "https://github.com/acme/shop",
		"ssh://git@github.com:22/acme/shop.git":        "https://github.com/acme/shop",
		"https://github.com/acme/shop.git":             "https://github.com/acme/shop",
		"https://user:token@github.com/acme/shop.git/": "https://github.com/acme/shop",
		"http://git.local:8080/acme/shop":              "http://git.local:8080/acme/shop",
		"/srv/git/shop.git":                            "",
		"file:///srv/git/shop.git":                     "",
		"../shop":                                      "",
	} {
		if got := webURL(remote); got != want {
			t.Errorf("webURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestNameFrom(t *testing.T) {
	for folder, want := range map[string]string{
		"shop-web": "shop-web", " Shop Web ": "Shop Web", "Über\tApp": "ÜberApp",
		strings.Repeat("a", 63) + " b": strings.Repeat("a", 63),
	} {
		if got := nameFrom(folder); got != want {
			t.Errorf("nameFrom(%q) = %q, want %q", folder, got, want)
		}
	}
}
