// Package config loads `tracks` preferences from
// ~/.config/tracks/config.yaml.
//
// A missing file is not an error — defaults are returned. Parse and
// validation errors ARE surfaced so users notice typos. Repos that the
// user wants `tracks` to operate on are listed here; per-track runtime
// state (worktree paths, status, etc.) lives in internal/state, not
// here.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the SchemaVersion that Default() writes.
// When we change the on-disk shape, bump this and add migration in
// Load(). Loaded files with a lower version go through migration;
// higher versions are refused so we don't corrupt forward-compatible
// fields the running binary doesn't understand.
const CurrentSchemaVersion = 1

// Config holds user preferences. New keys should default sensibly so
// an empty file still produces a working setup (except for repos,
// which the user has to enumerate).
type Config struct {
	SchemaVersion int    `yaml:"schema_version"`
	Tmux          Tmux   `yaml:"tmux,omitempty"`
	Paths         Paths  `yaml:"paths,omitempty"`
	Claude        Claude `yaml:"claude,omitempty"`
	Cursor        Cursor `yaml:"cursor,omitempty"`

	// Provider is the agent CLI new tracks default to. Empty means
	// Claude Code.
	//
	// Read today only by the daemon, when a creation request names no
	// provider of its own. The creation form will pre-select it and
	// allow a per-track override once that picker exists; there is no
	// UI for it yet.
	Provider string `yaml:"provider,omitempty"`
	Branch   Branch `yaml:"branch,omitempty"`
	Notify   Notify `yaml:"notify,omitempty"`
	Repos    []Repo `yaml:"repos,omitempty"`
}

// Tmux groups tmux-session settings.
type Tmux struct {
	// SessionName is the name of the tmux session `tracks` runs inside.
	// One session hosts the daemon, the dashboard, and one window per
	// running track.
	SessionName string `yaml:"session_name,omitempty"`

	// MenuKey is the key bound after the tmux prefix to open the
	// `tracks` menu popup. Defaults to "t". Changing this rebinds
	// `<prefix><key>` globally on the tmux server for the duration
	// of the session.
	MenuKey string `yaml:"menu_key,omitempty"`
}

// Paths groups filesystem locations. All entries support a leading
// `~/` which is expanded against the current user's home dir at load
// time (so the on-disk file remains portable across machines).
type Paths struct {
	// StateDir holds state.json, log files, and worktrees.
	// Default: ~/.local/state/tracks.
	StateDir string `yaml:"state_dir,omitempty"`

	// SocketDir holds the daemon's Unix socket. Empty means
	// auto-detect: $XDG_RUNTIME_DIR/tracks on Linux,
	// $TMPDIR/tracks-<uid> on macOS.
	SocketDir string `yaml:"socket_dir,omitempty"`
}

// Claude groups settings for how `tracks` invokes the claude binary.
type Claude struct {
	// Binary is the claude executable. Either a bare name (PATH lookup)
	// or an absolute path. Default: "claude".
	Binary string `yaml:"binary,omitempty"`

	// PermissionMode is passed as --permission-mode. Default: "auto".
	// Set to "acceptEdits" for stricter behavior or
	// "bypassPermissions" for full autonomy (be careful — Claude can
	// then run arbitrary commands in the worktree without asking).
	PermissionMode string `yaml:"permission_mode,omitempty"`

	// Model is passed as --model for a track that doesn't name its own.
	// Either an alias, which floats to the newest release in its family
	// ("opus", "sonnet", "haiku", "fable"), or a pinned id
	// ("claude-opus-4-8"), which does not. Empty omits the flag
	// entirely and leaves Claude on its own default.
	//
	// Note that a name the CLI doesn't recognise is not necessarily
	// rejected: "opus4.8" runs, silently, on a different model. Prefer
	// the picker or `tracks doctor` over typing an id here by hand.
	Model string `yaml:"model,omitempty"`

	// ModelByKind overrides Model for one kind of track, keyed by kind
	// ("work", "review", "ask", "plan", "doc") — a doc review rarely
	// needs the model a refactor does. An entry not naming a real kind
	// is inert.
	ModelByKind map[string]string `yaml:"model_by_kind,omitempty"`

	// ModelChoices is what the model picker offers at creation. Empty
	// falls back to DefaultModelChoices.
	//
	// The built-in list is aliases only, on purpose: an alias stays
	// correct as models come and go, while a pinned id in the binary
	// goes stale the way the price table does. Pins belong here, in the
	// user's own config, where they can be a deliberate choice.
	ModelChoices []ModelChoice `yaml:"model_choices,omitempty"`
}

// ModelChoice is one entry in the creation-time model picker.
type ModelChoice struct {
	// Label is what the picker shows. Empty falls back to Model.
	Label string `yaml:"label,omitempty"`

	// Model is the value passed to --model.
	Model string `yaml:"model"`
}

// providers are the agent CLIs a track may run on, and defaultProvider
// is the one new tracks get when the config says nothing.
//
// Duplicated from state.Provider rather than imported: internal/config
// and internal/state are deliberately independent of each other. The
// two lists are pinned together by a test in internal/daemon, which
// sees both — the same arrangement as modelKinds below.
var providers = []string{"claude", "cursor"}

const defaultProvider = "claude"

// Providers returns the agent CLIs a track may be created against.
func Providers() []string { return append([]string(nil), providers...) }

// modelKinds are the track kinds ModelByKind may be keyed by.
//
// Duplicated from state.Kind rather than imported: internal/config and
// internal/state are deliberately independent of each other. The two
// lists are pinned together by a test in internal/daemon, which sees
// both.
var modelKinds = []string{"work", "review", "ask", "plan", "doc"}

// ModelKinds returns the kinds a per-kind model override may name.
func ModelKinds() []string { return append([]string(nil), modelKinds...) }

// DefaultModelChoices is the built-in picker list: one alias per
// family, each of which follows that family's newest release.
func DefaultModelChoices() []ModelChoice {
	return []ModelChoice{
		{Label: "Opus (latest)", Model: "opus"},
		{Label: "Sonnet (latest)", Model: "sonnet"},
		{Label: "Haiku (latest)", Model: "haiku"},
		{Label: "Fable (latest)", Model: "fable"},
	}
}

// Choices returns the configured picker entries, or the built-in list
// when none are set.
func (c Claude) Choices() []ModelChoice {
	if len(c.ModelChoices) == 0 {
		return DefaultModelChoices()
	}
	return c.ModelChoices
}

// ModelFor returns the model a new track of the given kind should run,
// preferring a per-kind override over the global default. Empty means
// "pass no --model flag".
func (c Claude) ModelFor(kind string) string {
	if m, ok := c.ModelByKind[kind]; ok {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	return strings.TrimSpace(c.Model)
}

// Cursor groups settings for how `tracks` invokes the Cursor Agent
// binary. Deliberately shaped like Claude rather than shared with it:
// the two CLIs agree on more flags than they disagree on today, but a
// shared struct would have to grow a union of both as they drift.
type Cursor struct {
	// Binary is the Cursor Agent executable. Either a bare name (PATH
	// lookup) or an absolute path. Default: "agent".
	Binary string `yaml:"binary,omitempty"`

	// Model is passed as --model for a Cursor track that doesn't name
	// its own. Cursor's ids are its own — "gpt-5.3-codex",
	// "claude-opus-5-thinking-high", "composer-2.5" — and are NOT
	// interchangeable with the Claude API ids in Claude.Model. Empty
	// omits the flag, leaving Cursor's own default ("auto").
	Model string `yaml:"model,omitempty"`

	// ModelByKind overrides Model for one kind of track, keyed as in
	// Claude.ModelByKind.
	ModelByKind map[string]string `yaml:"model_by_kind,omitempty"`

	// ModelChoices is what the creation picker will offer for Cursor
	// tracks once that picker exists. Empty is intended to mean "ask the
	// binary" — `agent --list-models` is authoritative and
	// account-specific, so there is no useful built-in list to fall back
	// on the way there is for Claude.
	//
	// Nothing reads this yet.
	ModelChoices []ModelChoice `yaml:"model_choices,omitempty"`
}

// ModelFor returns the model a new Cursor track of the given kind
// should run, preferring a per-kind override over the global default.
// Empty means "pass no --model flag".
func (c Cursor) ModelFor(kind string) string {
	if m, ok := c.ModelByKind[kind]; ok {
		if m = strings.TrimSpace(m); m != "" {
			return m
		}
	}
	return strings.TrimSpace(c.Model)
}

// Notify controls how the daemon reaches out when a track wants
// attention. Each channel is independent; events gates which
// transitions trigger notifications at all.
type Notify struct {
	// MacOS enables the `osascript display notification` channel.
	// Silently a no-op on non-Darwin platforms.
	MacOS bool `yaml:"macos"`
	// Bell enables the terminal-bell channel (writes \a to
	// /dev/tty). tmux turns this into a status-line activity
	// marker on the window that fires.
	Bell bool `yaml:"bell"`
	// Events lists which transitions emit a notification. Valid values
	// are the notify.Event constants: track_created, waiting, done,
	// errored, pr_opened, pr_state_changed, service_ready,
	// service_failed. Empty defaults to the full set.
	Events []string `yaml:"events,omitempty"`
}

// EventEnabled reports whether the given event name is allowed by
// n.Events. An empty Events slice means "all enabled".
func (n Notify) EventEnabled(event string) bool {
	if len(n.Events) == 0 {
		return true
	}
	for _, e := range n.Events {
		if e == event {
			return true
		}
	}
	return false
}

// Branch controls the naming convention for branches the worktrees
// are created on.
type Branch struct {
	// Types are the allowed `<type>` prefixes (e.g. feat/fix/chore).
	// `tracks new` offers these in the picker.
	Types []string `yaml:"types,omitempty"`

	// DefaultType is the highlighted option in the picker. Must appear
	// in Types.
	DefaultType string `yaml:"default_type,omitempty"`
}

// Repo describes one repository that `tracks` may operate on.
type Repo struct {
	// Name is the short identifier shown in the picker. Must be unique
	// within the config.
	Name string `yaml:"name"`

	// Path is the absolute filesystem path to the primary checkout
	// (the one Cursor watches). Supports leading "~/".
	Path string `yaml:"path"`

	// Base is the branch every worktree branches off (e.g. "develop",
	// "main"). `tracks` runs `git fetch origin <base>` before creating
	// each worktree so the new branch starts from up-to-date code.
	Base string `yaml:"base"`

	// InitSubmodules opts the repo into a
	// `git submodule update --init --recursive` after each worktree
	// creation. Off by default because submodules can add minutes per
	// worktree.
	InitSubmodules bool `yaml:"init_submodules,omitempty"`

	// DraftPRs makes tracks for this repo open pull requests as drafts
	// by default. Surfaced to Claude as an instruction in the spawn
	// prompt (Claude opens PRs via `gh pr create`, so tracks can't force
	// the flag itself). Off by default.
	DraftPRs bool `yaml:"draft_prs,omitempty"`

	// Provision, when set, makes a freshly created worktree runnable:
	// it copies gitignored files (e.g. .env) from the primary checkout
	// and runs a dependency-install command. nil means no provisioning.
	Provision *Provision `yaml:"provision,omitempty"`

	// Services declares the dev servers a track for this repo can run,
	// lazy-started on demand via `tracks up <name>`. The binary stays
	// generic: anything repo-specific lives in the cmd/env/hooks here,
	// never hardcoded. Empty means no services.
	Services []Service `yaml:"services,omitempty"`
}

// Service is one named dev server a track can run. Cmd, Env values, and
// the hook commands are templated before launch (e.g. {{.Port "name"}}
// resolves to the port allocated to the service called "name"). The
// service runs as a supervised background process in the worktree.
type Service struct {
	// Name identifies the service within its repo (unique per repo). Used
	// by `tracks up <name>`, the tmux pane title, and port lookup.
	Name string `yaml:"name"`

	// Cmd is the shell command that starts the server. Templated.
	Cmd string `yaml:"cmd"`

	// Env are extra environment variables for Cmd, values templated and
	// merged onto the daemon's environment.
	Env map[string]string `yaml:"env,omitempty"`

	// Ready describes how to detect the service is up. At most one field
	// may be set; an empty probe means "ready as soon as it starts".
	Ready ReadyProbe `yaml:"ready,omitempty"`

	// PreStart, PostStart, and PreStop are shell commands run around the
	// service lifecycle (templated, in the worktree). Repo-specific wiring
	// — e.g. patching a live-app manifest URL with the allocated port —
	// lives here, not in the binary.
	PreStart  []string `yaml:"pre_start,omitempty"`
	PostStart []string `yaml:"post_start,omitempty"`
	PreStop   []string `yaml:"pre_stop,omitempty"`

	// DependsOn lists other services (same repo) that must be ready
	// before this one starts. A simple ordered wait, not a DAG.
	DependsOn []string `yaml:"depends_on,omitempty"`
}

// ReadyProbe is how a service signals readiness. At most one field set.
type ReadyProbe struct {
	// Port is satisfied when something is listening on this (templated)
	// TCP port, e.g. `{{.Port "live-app"}}` or a literal number.
	Port string `yaml:"port,omitempty"`

	// LogRegex is satisfied when the service's log output matches this
	// RE2 pattern, e.g. "compiled successfully".
	LogRegex string `yaml:"log_regex,omitempty"`
}

// IsZero reports whether the probe declares no readiness condition.
func (p ReadyProbe) IsZero() bool { return p.Port == "" && p.LogRegex == "" }

// Provision configures how a worktree is made runnable after creation:
// gitignored files are brought in from the primary checkout, then a
// dependency-install command is run. All fields are optional; a nil
// *Provision (the zero value on Repo) disables provisioning entirely.
type Provision struct {
	// DepsCmd is a shell command run in the new worktree to install
	// dependencies (e.g. "pnpm install --frozen-lockfile"). Empty skips
	// the install step.
	DepsCmd string `yaml:"deps_cmd,omitempty"`

	// CacheStrategy hints how dependencies are cached. "none" and
	// "pnpm-store" both just run DepsCmd (pnpm hardlinks from its store
	// natively); "apfs-clone" copy-on-write clones the primary's
	// node_modules into the worktree before DepsCmd so the install is an
	// incremental reconcile (best for yarn/npm repos without a global
	// store). Empty defaults to "none".
	CacheStrategy string `yaml:"cache_strategy,omitempty"`

	// CopyIgnored lists gitignored files to bring from the primary
	// checkout into the worktree. Entries are paths (or globs) relative
	// to the primary checkout root, e.g. ".env", "apps/*/.env.local".
	CopyIgnored []string `yaml:"copy_ignored,omitempty"`

	// CopyMode is how CopyIgnored entries are reproduced: "symlink"
	// (default) links back to the primary, "copy" makes an independent
	// copy. Empty defaults to "symlink".
	CopyMode string `yaml:"copy_mode,omitempty"`
}

// (The previous defaultPromptSuffix const lived here. The suffix
// is now hardcoded inside the daemon's claude spawn path so it
// can be edited and versioned with the binary instead of drifting
// through user YAML — see internal/claude/spawn.go.)

// Default returns a Config with documented defaults. The repos list
// is intentionally empty — the user must populate it for `tracks new`
// to have anything to pick from.
func Default() Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Tmux:          Tmux{SessionName: "tracks", MenuKey: "t"},
		Paths:         Paths{},
		Claude: Claude{
			Binary:         "claude",
			PermissionMode: "auto",
		},
		Cursor: Cursor{
			Binary: "agent",
		},
		Provider: defaultProvider,
		Branch: Branch{
			Types:       []string{"feat", "fix", "chore", "refactor", "docs", "test"},
			DefaultType: "fix",
		},
		Notify: Notify{
			MacOS:  true,
			Bell:   true,
			Events: nil, // nil = all events enabled (see EventEnabled).
		},
		Repos: nil,
	}
}

// Validate checks loaded values for internal consistency. It does not
// check that repo paths exist — that would make a missing checkout
// abort every command, which is overly hostile. Path existence is
// verified later, when a track actually wants to use a repo.
func (c Config) Validate() error {
	if c.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("schema_version %d is newer than this binary supports (max %d)",
			c.SchemaVersion, CurrentSchemaVersion)
	}
	if c.Tmux.SessionName == "" {
		return errors.New("tmux.session_name must not be empty")
	}
	if c.Claude.Binary == "" {
		return errors.New("claude.binary must not be empty")
	}
	if c.Cursor.Binary == "" {
		return errors.New("cursor.binary must not be empty")
	}

	// Branch types: at least one, no dupes, default must be present.
	if len(c.Branch.Types) == 0 {
		return errors.New("branch.types must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(c.Branch.Types))
	for _, t := range c.Branch.Types {
		if t == "" {
			return errors.New("branch.types contains an empty entry")
		}
		if _, dup := seen[t]; dup {
			return fmt.Errorf("branch.types contains duplicate %q", t)
		}
		seen[t] = struct{}{}
	}
	if c.Branch.DefaultType != "" {
		if _, ok := seen[c.Branch.DefaultType]; !ok {
			return fmt.Errorf("branch.default_type %q not in branch.types", c.Branch.DefaultType)
		}
	}

	// Model settings. A mistyped kind key ("docs" for "doc") is
	// otherwise inert and silent, and a choice with no model shows up in
	// the picker as a second, indistinguishable "Default" entry.
	validKind := make(map[string]struct{}, len(modelKinds))
	for _, k := range modelKinds {
		validKind[k] = struct{}{}
	}
	for kind := range c.Claude.ModelByKind {
		if _, ok := validKind[kind]; !ok {
			return fmt.Errorf("claude.model_by_kind has unknown kind %q (want one of %s)",
				kind, strings.Join(modelKinds, ", "))
		}
	}
	for i, ch := range c.Claude.ModelChoices {
		if strings.TrimSpace(ch.Model) == "" {
			return fmt.Errorf("claude.model_choices[%d] has an empty model", i)
		}
	}
	for kind := range c.Cursor.ModelByKind {
		if _, ok := validKind[kind]; !ok {
			return fmt.Errorf("cursor.model_by_kind has unknown kind %q (want one of %s)",
				kind, strings.Join(modelKinds, ", "))
		}
	}
	for i, ch := range c.Cursor.ModelChoices {
		if strings.TrimSpace(ch.Model) == "" {
			return fmt.Errorf("cursor.model_choices[%d] has an empty model", i)
		}
	}
	if p := strings.TrimSpace(c.Provider); p != "" && !slices.Contains(providers, p) {
		return fmt.Errorf("provider %q is not one of %s", p, strings.Join(providers, ", "))
	}

	// Repos: unique names, non-empty path and base.
	repoNames := make(map[string]struct{}, len(c.Repos))
	for i, r := range c.Repos {
		if r.Name == "" {
			return fmt.Errorf("repos[%d].name is required", i)
		}
		if _, dup := repoNames[r.Name]; dup {
			return fmt.Errorf("repos: duplicate name %q", r.Name)
		}
		repoNames[r.Name] = struct{}{}
		if r.Path == "" {
			return fmt.Errorf("repos[%s].path is required", r.Name)
		}
		if r.Base == "" {
			return fmt.Errorf("repos[%s].base is required", r.Name)
		}
		if p := r.Provision; p != nil {
			switch p.CacheStrategy {
			case "", "none", "pnpm-store", "apfs-clone":
				// ok
			default:
				return fmt.Errorf("repos[%s].provision.cache_strategy %q is invalid (want none, pnpm-store, or apfs-clone)", r.Name, p.CacheStrategy)
			}
			switch p.CopyMode {
			case "", "symlink", "copy":
				// ok
			default:
				return fmt.Errorf("repos[%s].provision.copy_mode %q is invalid (want symlink or copy)", r.Name, p.CopyMode)
			}
		}
		if err := validateServices(r.Name, r.Services); err != nil {
			return err
		}
	}

	return nil
}

// validateServices checks a repo's service definitions: unique non-empty
// names, a command, at most one readiness kind, and depends_on edges that
// reference real sibling services without cycles.
func validateServices(repoName string, services []Service) error {
	if len(services) == 0 {
		return nil
	}
	names := make(map[string]struct{}, len(services))
	for _, svc := range services {
		if svc.Name == "" {
			return fmt.Errorf("repos[%s].services: a service is missing a name", repoName)
		}
		if _, dup := names[svc.Name]; dup {
			return fmt.Errorf("repos[%s].services: duplicate name %q", repoName, svc.Name)
		}
		names[svc.Name] = struct{}{}
	}
	for _, svc := range services {
		if strings.TrimSpace(svc.Cmd) == "" {
			return fmt.Errorf("repos[%s].services[%s].cmd is required", repoName, svc.Name)
		}
		if svc.Ready.Port != "" && svc.Ready.LogRegex != "" {
			return fmt.Errorf("repos[%s].services[%s].ready: set at most one of port or log_regex", repoName, svc.Name)
		}
		for _, dep := range svc.DependsOn {
			if dep == svc.Name {
				return fmt.Errorf("repos[%s].services[%s] depends on itself", repoName, svc.Name)
			}
			if _, ok := names[dep]; !ok {
				return fmt.Errorf("repos[%s].services[%s].depends_on references unknown service %q", repoName, svc.Name, dep)
			}
		}
	}
	return dependencyCycle(repoName, services)
}

// dependencyCycle reports an error if the depends_on edges form a cycle,
// which would deadlock the ordered wait-for-ready at start time.
func dependencyCycle(repoName string, services []Service) error {
	deps := make(map[string][]string, len(services))
	for _, svc := range services {
		deps[svc.Name] = svc.DependsOn
	}
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(services))
	var visit func(name string) bool
	visit = func(name string) bool {
		switch state[name] {
		case visiting:
			return true
		case done:
			return false
		}
		state[name] = visiting
		for _, dep := range deps[name] {
			if visit(dep) {
				return true
			}
		}
		state[name] = done
		return false
	}
	for _, svc := range services {
		if visit(svc.Name) {
			return fmt.Errorf("repos[%s].services: depends_on cycle involving %q", repoName, svc.Name)
		}
	}
	return nil
}

// Path returns the canonical config file path. Honors $XDG_CONFIG_HOME
// when set, falls back to ~/.config/tracks/config.yaml.
func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tracks", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tracks", "config.yaml"), nil
}

// LegacyProxyEntry is a proxy_port declaration recovered from an older
// config file, used only to seed state on the one-time migration to
// state-owned proxies.
type LegacyProxyEntry struct {
	Service string
	Port    int
	BindAll bool
}

// LegacyProxyPorts re-parses the config file for the now-removed
// repos[].services[].proxy_port declarations. Returns nothing when the file
// is missing or carries none. Migration-only: the proxy_port field no longer
// exists on Service, so this tolerant re-read is the only place it is still
// seen. Remove once every state file has been upgraded.
func LegacyProxyPorts() []LegacyProxyEntry {
	p, err := Path()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var raw struct {
		Repos []struct {
			Services []struct {
				Name         string `yaml:"name"`
				ProxyPort    int    `yaml:"proxy_port"`
				ProxyBindAll bool   `yaml:"proxy_bind_all"`
			} `yaml:"services"`
		} `yaml:"repos"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	var out []LegacyProxyEntry
	for _, repo := range raw.Repos {
		for _, svc := range repo.Services {
			if svc.ProxyPort > 0 {
				out = append(out, LegacyProxyEntry{Service: svc.Name, Port: svc.ProxyPort, BindAll: svc.ProxyBindAll})
			}
		}
	}
	return out
}

// Load reads the config file at Path() and merges it onto Default().
// A missing file returns the defaults with no error. Parse and
// validation errors ARE surfaced.
//
// Merging-on-defaults means users can drop a partial config (e.g.
// just `repos:`) and still get sensible behavior for the rest.
func Load() (Config, error) {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", p, err)
	}
	// Defaults are merged via the unmarshal-onto-existing-struct trick
	// above, but YAML zeros out fields that the user explicitly set to
	// empty strings. Restore must-have defaults that ended up empty.
	if cfg.Tmux.SessionName == "" {
		cfg.Tmux.SessionName = "tracks"
	}
	if cfg.Tmux.MenuKey == "" {
		cfg.Tmux.MenuKey = "t"
	}
	if cfg.Claude.Binary == "" {
		cfg.Claude.Binary = "claude"
	}
	if cfg.Claude.PermissionMode == "" {
		cfg.Claude.PermissionMode = "auto"
	}
	if cfg.Cursor.Binary == "" {
		cfg.Cursor.Binary = "agent"
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = defaultProvider
	}
	if len(cfg.Branch.Types) == 0 {
		cfg.Branch.Types = Default().Branch.Types
	}
	if cfg.Branch.DefaultType == "" {
		cfg.Branch.DefaultType = "fix"
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", p, err)
	}
	return cfg, nil
}

// Save writes cfg to Path() as YAML, creating parent directories as
// needed. Existing files are overwritten atomically (temp + rename).
func Save(cfg Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".config.*.yaml")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return p, nil
}

// Init writes a default config file if one does not already exist.
// Returns the path written to and a bool indicating whether a new
// file was created (false if the file already existed).
func Init() (string, bool, error) {
	p, err := Path()
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(p); err == nil {
		return p, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return p, false, err
	}
	written, err := Save(Default())
	return written, err == nil, err
}

// expandHome expands a leading "~/" in p against the current user's
// home directory. Anything else is returned unchanged.
func expandHome(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p, err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	// "~user/..." form. Resolve via os/user.
	rest := strings.SplitN(p[1:], string(os.PathSeparator), 2)
	u, err := user.Lookup(rest[0])
	if err != nil {
		return p, err
	}
	if len(rest) == 1 {
		return u.HomeDir, nil
	}
	return filepath.Join(u.HomeDir, rest[1]), nil
}

// ResolveStateDir returns the absolute state dir to use, applying the
// config override (with "~/" expansion) or the default.
func (c Config) ResolveStateDir() (string, error) {
	if c.Paths.StateDir != "" {
		return expandHome(c.Paths.StateDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "tracks"), nil
}

// ResolveSocketDir returns the absolute socket dir to use. Honors the
// config override first; then the TRACKS_SOCKET_DIR env var; otherwise
// $XDG_RUNTIME_DIR/tracks on Linux, $TMPDIR/tracks-<uid> on macOS or
// anywhere XDG_RUNTIME_DIR isn't set.
//
// TRACKS_SOCKET_DIR is how a track's Claude pane finds the daemon: the
// daemon exports the dir it actually bound into the pane env, and the CLI
// honors it here. Without this, the CLI would recompute the dir from
// $TMPDIR/$XDG_RUNTIME_DIR — which on macOS can differ between the
// daemon's environment and a login shell's — and dial the wrong socket.
//
// This resolver serves both roles: "where to dial" (CLI) and "where to
// bind" (daemon, via SocketPath at startup). Honoring the env var is safe
// only because the var is injected per-pane (see spawn.go), never into the
// tmux global env, and the daemon is started via `tmux run-shell -b` in the
// server env — so the daemon never inherits a pane's value and binds by the
// heuristic below. Do not launch the daemon from inside a track pane, or it
// would bind to the injected dir instead of its own.
func (c Config) ResolveSocketDir() (string, error) {
	if c.Paths.SocketDir != "" {
		return expandHome(c.Paths.SocketDir)
	}
	if env := os.Getenv("TRACKS_SOCKET_DIR"); env != "" {
		return expandHome(env)
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "tracks"), nil
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	uid := strconv.Itoa(os.Getuid())
	return filepath.Join(tmp, "tracks-"+uid), nil
}

// ResolveRepoPath returns the absolute path of repo r with "~/"
// expansion applied. Does not stat — non-existent paths return
// without error so callers can decide how to react.
func (r Repo) ResolveRepoPath() (string, error) {
	return expandHome(r.Path)
}

// RepoByName looks up a repo by Name. Returns (zero, false) when not
// found.
func (c Config) RepoByName(name string) (Repo, bool) {
	for _, r := range c.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}
