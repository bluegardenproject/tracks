package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withXDGConfig points config.Path() at a temp dir for the duration of
// the test, so Save/Load don't touch the user's real ~/.config.
func withXDGConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default() failed Validate: %v", err)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	withXDGConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if cfg.Tmux.SessionName != "tracks" {
		t.Errorf("default session name = %q, want tracks", cfg.Tmux.SessionName)
	}
	if len(cfg.Repos) != 0 {
		t.Errorf("default repos = %v, want empty", cfg.Repos)
	}
}

func TestSaveThenLoadRoundtrip(t *testing.T) {
	withXDGConfig(t)
	in := Default()
	in.Repos = []Repo{
		{Name: "repo-a", Path: "/Users/x/code/repo-a", Base: "develop"},
		{Name: "repo-b", Path: "/Users/x/code/repo-b", Base: "develop", InitSubmodules: true},
	}
	if _, err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.Repos) != 2 || out.Repos[1].Name != "repo-b" || !out.Repos[1].InitSubmodules {
		t.Errorf("roundtrip lost data: %+v", out.Repos)
	}
}

func TestLoadPartialMergesDefaults(t *testing.T) {
	dir := withXDGConfig(t)
	// User writes only `repos:` and leaves everything else missing.
	yaml := `schema_version: 1
repos:
  - name: lumen
    path: /tmp/lumen
    base: main
`
	cfgFile := filepath.Join(dir, "tracks", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgFile, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tmux.SessionName != "tracks" {
		t.Errorf("session_name not defaulted: %q", cfg.Tmux.SessionName)
	}
	if cfg.Claude.PermissionMode != "auto" {
		t.Errorf("permission_mode not defaulted: %q", cfg.Claude.PermissionMode)
	}
	if cfg.Branch.DefaultType != "fix" {
		t.Errorf("default_type not defaulted: %q", cfg.Branch.DefaultType)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "lumen" {
		t.Errorf("repos not loaded: %+v", cfg.Repos)
	}
}

func TestValidateRejectsBadBranchDefault(t *testing.T) {
	c := Default()
	c.Branch.DefaultType = "nope"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown default_type")
	}
}

func TestValidateRejectsDuplicateRepos(t *testing.T) {
	c := Default()
	c.Repos = []Repo{
		{Name: "x", Path: "/a", Base: "main"},
		{Name: "x", Path: "/b", Base: "main"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestValidateRejectsNewerSchema(t *testing.T) {
	c := Default()
	c.SchemaVersion = CurrentSchemaVersion + 1
	if err := c.Validate(); err == nil {
		t.Fatal("expected schema-version error")
	}
}

func TestResolveStateDirHomeExpansion(t *testing.T) {
	c := Default()
	c.Paths.StateDir = "~/foo/bar"
	got, err := c.ResolveStateDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSocketDirXDGRuntime(t *testing.T) {
	c := Default()
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	got, err := c.ResolveSocketDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/run/user/1000/tracks" {
		t.Errorf("got %q", got)
	}
}

func TestResolveSocketDirTMPDIRFallback(t *testing.T) {
	c := Default()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/tmp")
	got, err := c.ResolveSocketDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "/tmp/tracks-") {
		t.Errorf("got %q, want /tmp/tracks-<uid>", got)
	}
}

func TestRepoByName(t *testing.T) {
	c := Default()
	c.Repos = []Repo{{Name: "a", Path: "/a", Base: "main"}}
	if _, ok := c.RepoByName("a"); !ok {
		t.Error("not found")
	}
	if _, ok := c.RepoByName("b"); ok {
		t.Error("unexpectedly found")
	}
}
