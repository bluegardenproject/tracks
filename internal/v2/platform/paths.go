// Package platform resolves where Tracks v2 keeps its files and which
// tmux server it runs on. Every v2 location differs from v1's, so both
// apps can run side by side.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// Profile selects a set of locations.
type Profile int

const (
	// Default is the real app.
	Default Profile = iota
	// Demo is the playground: its own tmux server and a throwaway data
	// directory.
	Demo
)

// Paths are the locations for one profile.
type Paths struct {
	// ConfigDir holds config.yaml and user overrides. Shared by all
	// profiles.
	ConfigDir string
	// DataDir holds state, logs and generated files.
	DataDir string
	// Database is the SQLite database. Shared by all profiles: the
	// playground's fake tracks never reach it, its repos are real.
	Database string
	// TmuxSocket is the tmux socket name (`tmux -L`).
	TmuxSocket string
}

// Resolve returns the paths for profile in the current environment.
func Resolve(profile Profile) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find home directory: %w", err)
	}
	return resolve(profile, env{
		home:      home,
		xdgConfig: os.Getenv("XDG_CONFIG_HOME"),
		xdgState:  os.Getenv("XDG_STATE_HOME"),
		tempDir:   os.TempDir(),
		uid:       os.Getuid(),
	}), nil
}

type env struct {
	home, xdgConfig, xdgState, tempDir string
	uid                                int
}

func resolve(profile Profile, e env) Paths {
	configHome := e.xdgConfig
	if configHome == "" {
		configHome = filepath.Join(e.home, ".config")
	}
	stateHome := e.xdgState
	if stateHome == "" {
		stateHome = filepath.Join(e.home, ".local", "state")
	}
	state := filepath.Join(stateHome, "tracks-v2")
	p := Paths{ConfigDir: filepath.Join(configHome, "tracks-v2"), Database: filepath.Join(state, "tracks.db")}

	switch profile {
	case Demo:
		p.DataDir = filepath.Join(e.tempDir, fmt.Sprintf("tracks-v2-demo-%d", e.uid))
		p.TmuxSocket = "tracks-v2-demo"
	default:
		p.DataDir = state
		p.TmuxSocket = "tracks-v2"
	}
	return p
}
