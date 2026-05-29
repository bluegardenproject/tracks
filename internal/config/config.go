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
	Branch        Branch `yaml:"branch,omitempty"`
	Repos         []Repo `yaml:"repos,omitempty"`
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
		Branch: Branch{
			Types:       []string{"feat", "fix", "chore", "refactor", "docs", "test"},
			DefaultType: "fix",
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
// config override; otherwise $XDG_RUNTIME_DIR/tracks on Linux,
// $TMPDIR/tracks-<uid> on macOS or anywhere XDG_RUNTIME_DIR isn't set.
func (c Config) ResolveSocketDir() (string, error) {
	if c.Paths.SocketDir != "" {
		return expandHome(c.Paths.SocketDir)
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
