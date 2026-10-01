package agents

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/track"
)

// ErrNoRepos is returned for a work or review track without repos.
var ErrNoRepos = errors.New("track has no repos")

// Spec is what an engine needs to start on a track.
type Spec struct {
	Track track.Track
	// Program is the engine's CLI: a path, or a name on PATH.
	Program string
	// Auto is the engine's auto mode, from the Engines tab.
	Auto bool
	// Resume continues the track's session: the session brings back its
	// prompt and model, so neither is passed.
	Resume bool
	// DraftPRs names the track's repos that open pull requests as
	// drafts.
	DraftPRs []string
	// SocketDir is where the daemon listens, and BinDir goes first on
	// the pane's PATH.
	SocketDir, BinDir string
	// Hooks is the track's hooks, as the engine takes them: Claude's
	// settings file, Cursor's plugin folder; "" for none.
	Hooks string
}

// Start is the command a track's agent pane runs, and the folder it
// starts in.
type Start struct{ Command, Dir string }

// Wrap puts line in the pane scaffolding for s's track.
func (s Spec) Wrap(line CommandLine) string {
	return Wrapper{TrackID: s.Track.ID, SocketDir: s.SocketDir, BinDir: s.BinDir}.Command(line)
}

// DocDir is the folder a doc track works in: the document itself when
// it is a folder, else the folder holding it. Empty for other kinds.
func DocDir(t track.Track) string {
	if t.Kind != track.Doc || t.Document == "" {
		return ""
	}
	if info, err := os.Stat(t.Document); err == nil && info.IsDir() {
		return t.Document
	}
	return filepath.Dir(t.Document)
}

// Home is the folder a pane opens in when a track has nothing better.
func Home() string {
	home, _ := os.UserHomeDir()
	return home
}
