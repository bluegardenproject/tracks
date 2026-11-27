package plugins

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/plugin"
	"github.com/bluegardenproject/tracks/internal/store"
)

const fixture = "../plugin/testdata/acme"

// isolate keeps git away from the user's config.
func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo makes a git repository holding the fixture plugin and returns
// its folder.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "--quiet", "--initial-branch=main")
	commit(t, dir, "the plugin")
	return dir
}

func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "--quiet", "-m", msg)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func edit(t *testing.T, dir, old, next string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, plugin.File))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("no %q in the manifest", old)
	}
	write(t, dir, plugin.File, strings.Replace(string(b), old, next, 1))
}

func service(t *testing.T) Service {
	t.Helper()
	isolate(t)
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tracks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return Service{Store: db, Dir: filepath.Join(os.Getenv("XDG_STATE_HOME"), "tracks", "plugins")}
}

func yes(Proposal) (bool, error) { return true, nil }

func src(url string) Source { return Source{URL: "file://" + url} }

func TestAddFromGit(t *testing.T) {
	s, ctx := service(t), context.Background()
	r := repo(t)
	head := gitIn(t, r, "rev-parse", "HEAD")
	var seen Proposal
	p, err := s.Add(ctx, src(r), func(p Proposal) (bool, error) { seen = p; return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	if seen.Commit != head || seen.Manifest.Name != "acme-planning" ||
		!strings.Contains(seen.Approval.Describe(), "ctx/adrs (context provider of custom-planner)") {
		t.Errorf("proposal %+v", seen)
	}
	want := filepath.Join(s.Dir, "acme-planning", head)
	if p.Dir != want || p.Pinned != head || p.Ref != "" || !p.Enabled || p.Local || p.Source != "file://"+r {
		t.Errorf("installed %+v", p)
	}
	if _, err := os.Stat(filepath.Join(want, "briefs", "research.md")); err != nil {
		t.Error(err)
	}
	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Approval.Summary() != "track custom-planner, train research" {
		t.Errorf("list %+v", list)
	}
	if _, err := s.Add(ctx, src(r), yes); err == nil || !strings.Contains(err.Error(), "installed already") {
		t.Errorf("added twice: %v", err)
	}
	noStaging(t, s)
}

func TestAddAtRef(t *testing.T) {
	s, ctx := service(t), context.Background()
	r := repo(t)
	first := gitIn(t, r, "rev-parse", "HEAD")
	gitIn(t, r, "tag", "v1")
	write(t, r, "extra.txt", "x")
	commit(t, r, "more")
	p, err := s.Add(ctx, Source{URL: "file://" + r, Ref: "v1"}, yes)
	if err != nil {
		t.Fatal(err)
	}
	if p.Pinned != first || p.Ref != "v1" {
		t.Errorf("installed %+v", p)
	}
	if _, err := s.Add(ctx, Source{URL: "file://" + r, Ref: "nope"}, yes); err == nil || !strings.Contains(err.Error(), "no branch, tag or commit nope") {
		t.Errorf("unknown ref: %v", err)
	}
}

func TestAddDeclined(t *testing.T) {
	s, ctx := service(t), context.Background()
	_, err := s.Add(ctx, src(repo(t)), func(Proposal) (bool, error) { return false, nil })
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err %v", err)
	}
	if list, _ := s.List(ctx); len(list) != 0 {
		t.Errorf("installed %+v", list)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "acme-planning")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("folder left: %v", err)
	}
	noStaging(t, s)
}

func TestAddInvalid(t *testing.T) {
	s, ctx := service(t), context.Background()
	r := repo(t)
	edit(t, r, "permissions: read", "permissions: root")
	commit(t, r, "bad")
	_, err := s.Add(ctx, src(r), func(Proposal) (bool, error) { t.Fatal("asked to approve"); return false, nil })
	if err == nil || !strings.Contains(err.Error(), "components[1].permissions") {
		t.Errorf("err %v", err)
	}
	noStaging(t, s)
}

func TestUpdate(t *testing.T) {
	s, ctx := service(t), context.Background()
	r := repo(t)
	p, err := s.Add(ctx, src(r), yes)
	if err != nil {
		t.Fatal(err)
	}
	asked := func(c *Change) func(Change) (bool, error) {
		return func(got Change) (bool, error) { *c = got; return true, nil }
	}

	var c Change
	if c, err = s.Update(ctx, "acme-planning", asked(&c)); err != nil || !c.Current {
		t.Fatalf("no change: %+v, %v", c, err)
	}

	write(t, r, "briefs/research.md", "A better brief.\n")
	next := commit(t, r, "brief")
	if _, err := s.Update(ctx, "acme-planning", asked(&c)); err != nil {
		t.Fatal(err)
	}
	if c.Ask || c.From != p.Pinned || c.To != next || len(c.Code) > 0 || c.ManifestDiff != "" {
		t.Errorf("brief: %+v", c)
	}
	if _, err := os.Stat(p.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old commit's folder left: %v", err)
	}

	write(t, r, "servers/jira/server.sh", "#!/bin/sh\necho changed\n")
	commit(t, r, "code")
	if _, err := s.Update(ctx, "acme-planning", asked(&c)); err != nil {
		t.Fatal(err)
	}
	if !c.Ask || !slices.Equal(c.Code, []string{"servers/jira/server.sh"}) || len(c.Widens) > 0 {
		t.Errorf("code: %+v", c)
	}

	edit(t, r, "permissions: read", "permissions: write")
	commit(t, r, "write")
	before, _ := s.Store.Plugin(ctx, "acme-planning")
	_, err = s.Update(ctx, "acme-planning", func(got Change) (bool, error) { c = got; return false, nil })
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("declined: %v", err)
	}
	if !c.Ask || !slices.Equal(c.Widens, []string{"research: permissions write, were read"}) ||
		!strings.Contains(c.ManifestDiff, "+    permissions: write") {
		t.Errorf("permissions: %+v", c)
	}
	after, _ := s.Store.Plugin(ctx, "acme-planning")
	if after.Pinned != before.Pinned || after.Approved != before.Approved {
		t.Errorf("declined update changed %+v to %+v", before, after)
	}
	if _, err := os.Stat(after.Dir); err != nil {
		t.Errorf("installed folder: %v", err)
	}
	noStaging(t, s)
}

func TestUpdateRenamed(t *testing.T) {
	s, ctx := service(t), context.Background()
	r := repo(t)
	if _, err := s.Add(ctx, src(r), yes); err != nil {
		t.Fatal(err)
	}
	edit(t, r, "name: acme-planning", "name: acme")
	commit(t, r, "rename")
	if _, err := s.Update(ctx, "acme-planning", func(Change) (bool, error) { return true, nil }); err == nil ||
		!strings.Contains(err.Error(), "now called acme") {
		t.Errorf("err %v", err)
	}
}

func TestLocal(t *testing.T) {
	s, ctx := service(t), context.Background()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(ctx, Source{Local: dir}, yes)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.Dir, "acme-planning", "local")
	if target, err := os.Readlink(link); err != nil || !p.Local || p.Pinned != "" || p.Dir != link {
		t.Fatalf("installed %+v, link %s %v", p, target, err)
	}
	var c Change
	if c, err = s.Update(ctx, "acme-planning", func(Change) (bool, error) { t.Fatal("asked"); return false, nil }); err != nil || !c.Current {
		t.Fatalf("unchanged: %+v %v", c, err)
	}
	edit(t, dir, `hosts: ["*.atlassian.net"]`, `hosts: ["*.atlassian.net", "api.github.com"]`)
	if _, err := s.Update(ctx, "acme-planning", func(got Change) (bool, error) { c = got; return true, nil }); err != nil {
		t.Fatal(err)
	}
	if !c.Ask || !c.Local || !slices.Equal(c.Widens, []string{"research: hosts api.github.com"}) {
		t.Errorf("change %+v", c)
	}
	if err := s.Remove(ctx, "acme-planning"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, plugin.File)); err != nil {
		t.Errorf("removing a local plugin touched its folder: %v", err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("link left: %v", err)
	}
	if _, err := s.Add(ctx, Source{Local: "relative"}, yes); err == nil {
		t.Error("added a relative path")
	}
}

func TestRemoveAndEnable(t *testing.T) {
	s, ctx := service(t), context.Background()
	p, err := s.Add(ctx, src(repo(t)), yes)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, "acme-planning", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Store.Plugin(ctx, "acme-planning"); got.Enabled {
		t.Error("still enabled")
	}
	if err := s.Remove(ctx, "acme-planning"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(p.Dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("folder left: %v", err)
	}
	for _, err := range []error{s.Remove(ctx, "acme-planning"), s.SetEnabled(ctx, "acme-planning", true)} {
		if err == nil || !strings.Contains(err.Error(), "no plugin called acme-planning") {
			t.Errorf("gone: %v", err)
		}
	}
	if _, err := s.Update(ctx, "../x", nil); err == nil {
		t.Error("updated ../x")
	}
}

func noStaging(t *testing.T, s Service) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(s.Dir, ".staging-*"))
	if len(left) > 0 {
		t.Errorf("staging left: %v", left)
	}
}
