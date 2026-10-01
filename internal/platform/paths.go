// Package platform resolves where Tracks keeps its files and which tmux
// server it runs on.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths are where Tracks keeps its files.
type Paths struct {
	// ConfigDir holds the settings, the user's themes and overrides.
	ConfigDir string
	// Settings is the preferences file, and ThemesDir holds the user's
	// themes, one file each.
	Settings, ThemesDir string
	// DataDir holds state, logs and generated files.
	DataDir string
	// Database is the SQLite database.
	Database string
	// Worktrees holds the tracks' worktrees, a folder per track.
	Worktrees string
	// Socket, Lock and Log are the daemon's; BinDir goes first on a
	// track's PATH.
	Socket, Lock, Log, BinDir string
	// TmuxSocket is the tmux socket name (`tmux -L`).
	TmuxSocket string
}

// Resolve returns the paths in the current environment.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find home directory: %w", err)
	}
	return resolve(env{
		home:      home,
		xdgConfig: os.Getenv("XDG_CONFIG_HOME"),
		xdgState:  os.Getenv("XDG_STATE_HOME"),
	}), nil
}

type env struct{ home, xdgConfig, xdgState string }

func resolve(e env) Paths {
	configHome := e.xdgConfig
	if configHome == "" {
		configHome = filepath.Join(e.home, ".config")
	}
	stateHome := e.xdgState
	if stateHome == "" {
		stateHome = filepath.Join(e.home, ".local", "state")
	}
	state := filepath.Join(stateHome, "tracks")
	config := filepath.Join(configHome, "tracks")
	return Paths{
		ConfigDir:  config,
		Settings:   filepath.Join(config, "settings.yaml"),
		ThemesDir:  filepath.Join(config, "themes"),
		DataDir:    state,
		Database:   filepath.Join(state, "tracks.db"),
		Worktrees:  filepath.Join(state, "worktrees"),
		Socket:     filepath.Join(state, "daemon.sock"),
		Lock:       filepath.Join(state, "daemon.lock"),
		Log:        filepath.Join(state, "daemon.log"),
		BinDir:     filepath.Join(state, "bin"),
		TmuxSocket: "tracks",
	}
}
