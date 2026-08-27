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

func TestProvisionRoundtrip(t *testing.T) {
	withXDGConfig(t)
	in := Default()
	in.Repos = []Repo{
		{
			Name: "repo-a", Path: "/Users/x/code/repo-a", Base: "develop",
			Provision: &Provision{
				DepsCmd:       "pnpm install --frozen-lockfile",
				CacheStrategy: "pnpm-store",
				CopyIgnored:   []string{".env", ".env.local"},
				CopyMode:      "copy",
			},
		},
		// A repo without provisioning must round-trip back to nil.
		{Name: "repo-b", Path: "/Users/x/code/repo-b", Base: "develop"},
	}
	if _, err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := out.Repos[0].Provision
	if a == nil {
		t.Fatal("repo-a provision lost on roundtrip")
	}
	if a.DepsCmd != "pnpm install --frozen-lockfile" || a.CacheStrategy != "pnpm-store" ||
		a.CopyMode != "copy" || len(a.CopyIgnored) != 2 || a.CopyIgnored[1] != ".env.local" {
		t.Errorf("provision roundtrip mismatch: %+v", a)
	}
	if out.Repos[1].Provision != nil {
		t.Errorf("repo-b provision should be nil, got %+v", out.Repos[1].Provision)
	}
}

func TestValidateRejectsBadProvision(t *testing.T) {
	cases := map[string]*Provision{
		"bad cache strategy": {CacheStrategy: "bogus"},
		"bad copy mode":      {CopyMode: "hardlink"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			c := Default()
			c.Repos = []Repo{{Name: "r", Path: "/a", Base: "main", Provision: p}}
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestValidateAcceptsMinimalProvision(t *testing.T) {
	c := Default()
	c.Repos = []Repo{{
		Name: "r", Path: "/a", Base: "main",
		Provision: &Provision{DepsCmd: "yarn"},
	}}
	if err := c.Validate(); err != nil {
		t.Fatalf("minimal provision should validate: %v", err)
	}
}

func TestValidateAcceptsApfsClone(t *testing.T) {
	c := Default()
	c.Repos = []Repo{{
		Name: "r", Path: "/a", Base: "main",
		Provision: &Provision{DepsCmd: "yarn", CacheStrategy: "apfs-clone"},
	}}
	if err := c.Validate(); err != nil {
		t.Fatalf("apfs-clone should validate: %v", err)
	}
}

func TestValidateRejectsBadServices(t *testing.T) {
	cases := map[string][]Service{
		"missing name":     {{Cmd: "run"}},
		"duplicate name":   {{Name: "a", Cmd: "x"}, {Name: "a", Cmd: "y"}},
		"missing cmd":      {{Name: "a"}},
		"two ready kinds":  {{Name: "a", Cmd: "x", Ready: ReadyProbe{Port: "1", LogRegex: "up"}}},
		"unknown dep":      {{Name: "a", Cmd: "x", DependsOn: []string{"ghost"}}},
		"self dep":         {{Name: "a", Cmd: "x", DependsOn: []string{"a"}}},
		"dependency cycle": {{Name: "a", Cmd: "x", DependsOn: []string{"b"}}, {Name: "b", Cmd: "y", DependsOn: []string{"a"}}},
	}
	for name, svcs := range cases {
		t.Run(name, func(t *testing.T) {
			c := Default()
			c.Repos = []Repo{{Name: "r", Path: "/a", Base: "main", Services: svcs}}
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestValidateAcceptsServices(t *testing.T) {
	c := Default()
	c.Repos = []Repo{{
		Name: "r", Path: "/a", Base: "main",
		Services: []Service{
			{Name: "lld", Cmd: "pnpm dev:lld", Ready: ReadyProbe{LogRegex: "compiled"}},
			{Name: "live-app", Cmd: `pnpm dev --port {{.Port "live-app"}}`, Ready: ReadyProbe{Port: `{{.Port "live-app"}}`}, DependsOn: []string{"lld"}},
		},
	}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid services should pass: %v", err)
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
	t.Setenv("TRACKS_SOCKET_DIR", "")
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
	t.Setenv("TRACKS_SOCKET_DIR", "")
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

// TRACKS_SOCKET_DIR is how a track's Claude pane is told where the daemon
// actually bound its socket; it must win over the XDG/TMPDIR heuristics.
func TestResolveSocketDirEnvVarWinsOverHeuristics(t *testing.T) {
	c := Default()
	t.Setenv("TRACKS_SOCKET_DIR", "/daemon/real/sockdir")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("TMPDIR", "/tmp")
	got, err := c.ResolveSocketDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/daemon/real/sockdir" {
		t.Errorf("got %q, want the TRACKS_SOCKET_DIR value", got)
	}
}

// An explicit config override is more specific than the injected env var,
// so it stays the top priority.
func TestResolveSocketDirConfigOverrideWinsOverEnv(t *testing.T) {
	c := Default()
	c.Paths.SocketDir = "/config/override"
	t.Setenv("TRACKS_SOCKET_DIR", "/daemon/real/sockdir")
	got, err := c.ResolveSocketDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/config/override" {
		t.Errorf("got %q, want the config override", got)
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

// A config carrying the removed proxy_port field still loads (unknown keys
// are ignored) and validates — the field is gone but old files don't break.
func TestConfigWithLegacyProxyPortStillLoads(t *testing.T) {
	dir := withXDGConfig(t)
	writeConfigYAML(t, dir, ""+
		"repos:\n"+
		"  - name: buy-sell\n"+
		"    path: /tmp/bs\n"+
		"    base: main\n"+
		"    services:\n"+
		"      - name: dev\n"+
		"        cmd: pnpm dev\n"+
		"        proxy_port: 3000\n")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("config with legacy proxy_port must still load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// LegacyProxyPorts recovers proxy_port declarations from an older config so
// the daemon can seed them into state once.
func TestLegacyProxyPorts(t *testing.T) {
	dir := withXDGConfig(t)
	writeConfigYAML(t, dir, ""+
		"repos:\n"+
		"  - name: buy-sell\n"+
		"    path: /tmp/bs\n"+
		"    base: main\n"+
		"    services:\n"+
		"      - name: dev\n"+
		"        cmd: pnpm dev\n"+
		"        proxy_port: 3000\n"+
		"      - name: metro\n"+
		"        cmd: pnpm start\n"+
		"        proxy_port: 8081\n"+
		"        proxy_bind_all: true\n"+
		"      - name: worker\n"+
		"        cmd: pnpm work\n")
	got := LegacyProxyPorts()
	if len(got) != 2 {
		t.Fatalf("got %d legacy ports, want 2: %+v", len(got), got)
	}
	if got[0].Port != 3000 || got[0].Service != "dev" || got[0].BindAll {
		t.Errorf("entry[0] = %+v, want dev :3000 loopback", got[0])
	}
	if got[1].Port != 8081 || !got[1].BindAll {
		t.Errorf("entry[1] = %+v, want metro :8081 bind-all", got[1])
	}
}

func writeConfigYAML(t *testing.T, xdgDir, body string) {
	t.Helper()
	dir := filepath.Join(xdgDir, "tracks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModelForPrefersThePerKindOverride(t *testing.T) {
	c := Claude{
		Model:       "claude-opus-5",
		ModelByKind: map[string]string{"doc": "haiku", "review": "  "},
	}
	if got := c.ModelFor("doc"); got != "haiku" {
		t.Errorf("ModelFor(doc) = %q, want the override", got)
	}
	if got := c.ModelFor("work"); got != "claude-opus-5" {
		t.Errorf("ModelFor(work) = %q, want the global default", got)
	}
	// A blank override is not a choice — it must not shadow the default
	// with an empty string, which would drop the flag entirely.
	if got := c.ModelFor("review"); got != "claude-opus-5" {
		t.Errorf("ModelFor(review) = %q, want a whitespace override ignored", got)
	}
}

func TestModelForEmptyWhenNothingConfigured(t *testing.T) {
	if got := (Claude{}).ModelFor("work"); got != "" {
		t.Errorf("ModelFor = %q, want empty so no --model flag is passed", got)
	}
}

// The built-in list is aliases only: a pinned id baked into the binary
// goes stale, an alias does not.
func TestDefaultChoicesAreAliasesOnly(t *testing.T) {
	// The known family aliases. A hyphen test would be a proxy for this
	// and would fire on a legitimate hyphenated alias.
	aliases := map[string]bool{"opus": true, "sonnet": true, "haiku": true, "fable": true, "mythos": true}
	for _, c := range (Claude{}).Choices() {
		if !aliases[c.Model] {
			t.Errorf("built-in choice %q is not a family alias; a pinned id in the binary goes stale", c.Model)
		}
		if c.Label == "" {
			t.Errorf("built-in choice %q has no label", c.Model)
		}
	}
}

func TestChoicesUsesTheConfiguredListWhenSet(t *testing.T) {
	c := Claude{ModelChoices: []ModelChoice{{Label: "Opus 4.8", Model: "claude-opus-4-8"}}}
	got := c.Choices()
	if len(got) != 1 || got[0].Model != "claude-opus-4-8" {
		t.Errorf("Choices() = %+v, want only the configured entry", got)
	}
}

func TestValidateRejectsAnUnknownKindKey(t *testing.T) {
	cfg := Default()
	cfg.Repos = []Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
	cfg.Claude.ModelByKind = map[string]string{"docs": "haiku"} // "doc", not "docs"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("a mistyped kind key was accepted; it would be silently inert")
	}
	if !strings.Contains(err.Error(), "docs") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
}

func TestValidateAcceptsEveryRealKind(t *testing.T) {
	cfg := Default()
	cfg.Repos = []Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
	cfg.Claude.ModelByKind = map[string]string{}
	for _, k := range ModelKinds() {
		cfg.Claude.ModelByKind[k] = "haiku"
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("all real kinds should validate, got: %v", err)
	}
}

func TestValidateRejectsAChoiceWithNoModel(t *testing.T) {
	cfg := Default()
	cfg.Repos = []Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
	cfg.Claude.ModelChoices = []ModelChoice{{Label: "Mystery", Model: "  "}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("an empty model was accepted; the picker would show a second indistinguishable Default")
	}
}

// Cursor.ModelFor mirrors Claude.ModelFor, so it gets the mirrored
// tests — the repo's pattern is that a copied struct copies its
// coverage, not that a copy is assumed correct.
func TestCursorModelForPrefersThePerKindOverride(t *testing.T) {
	c := Cursor{
		Model:       "gpt-5.3-codex",
		ModelByKind: map[string]string{"doc": "composer-2.5", "review": "  "},
	}
	if got := c.ModelFor("doc"); got != "composer-2.5" {
		t.Errorf("ModelFor(doc) = %q, want the override", got)
	}
	if got := c.ModelFor("work"); got != "gpt-5.3-codex" {
		t.Errorf("ModelFor(work) = %q, want the global default", got)
	}
	if got := c.ModelFor("review"); got != "gpt-5.3-codex" {
		t.Errorf("ModelFor(review) = %q, want a whitespace override ignored", got)
	}
}

func TestCursorModelForEmptyWhenNothingConfigured(t *testing.T) {
	if got := (Cursor{}).ModelFor("work"); got != "" {
		t.Errorf("ModelFor = %q, want empty so no --model flag is passed", got)
	}
}

func TestValidateRejectsCursorMisconfiguration(t *testing.T) {
	base := func() Config {
		c := Default()
		c.Repos = []Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
		return c
	}
	cases := []struct {
		name string
		bend func(*Config)
		want string
	}{
		{"unknown kind key", func(c *Config) { c.Cursor.ModelByKind = map[string]string{"docs": "auto"} }, "docs"},
		{"empty choice model", func(c *Config) { c.Cursor.ModelChoices = []ModelChoice{{Label: "X", Model: " "}} }, "cursor.model_choices"},
		{"empty binary", func(c *Config) { c.Cursor.Binary = "" }, "cursor.binary"},
		{"unknown provider", func(c *Config) { c.Provider = "codex" }, "codex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.bend(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("accepted an invalid config")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateAcceptsEveryOfferedProvider(t *testing.T) {
	for _, p := range Providers() {
		cfg := Default()
		cfg.Repos = []Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
		cfg.Provider = p
		if err := cfg.Validate(); err != nil {
			t.Errorf("Providers() offers %q but Validate rejects it: %v", p, err)
		}
	}
}

// A config that never mentions cursor or a provider still has to come
// out usable — this is every config written before the field existed.
func TestDefaultsFillInCursorAndProvider(t *testing.T) {
	cfg := Default()
	if cfg.Cursor.Binary != "agent" {
		t.Errorf("default cursor binary = %q, want agent", cfg.Cursor.Binary)
	}
	if cfg.Provider != "claude" {
		t.Errorf("default provider = %q, want claude", cfg.Provider)
	}
}
