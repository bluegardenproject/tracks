package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

const (
	sessionName  = "tracks"
	tracksWindow = "Tracks"
)

type startAction int

const (
	startCreate startAction = iota // create the session, then attach
	startAttach                    // attach another client
	startSelect                    // already inside: show the Tracks window
	startRefuse                    // inside another tmux server
)

func planStartup(loc tmux.Location, sessionExists bool) startAction {
	switch {
	case loc == tmux.OtherServer:
		return startRefuse
	case loc == tmux.OwnServer:
		return startSelect
	case sessionExists:
		return startAttach
	default:
		return startCreate
	}
}

var errInsideTmux = errors.New("you're inside another tmux session.\n" +
	"Tracks runs on its own tmux server. Open a new terminal tab and run it there.")

// start opens Tracks for profile: it starts the tmux server when needed
// and attaches this terminal to it.
func start(profile platform.Profile) error {
	version, err := tmux.InstalledVersion()
	if err != nil {
		return err
	}
	paths, err := platform.Resolve(profile)
	if err != nil {
		return err
	}
	c := tmux.New(paths.TmuxSocket)

	switch planStartup(tmux.LocationOf(os.Getenv("TMUX"), paths.TmuxSocket), c.HasSession(sessionName)) {
	case startRefuse:
		return errInsideTmux
	case startSelect:
		return c.SelectWindow(sessionName, "0")
	case startCreate:
		if err := createSession(c, paths, version); err != nil {
			return err
		}
	}
	return c.Attach(sessionName)
}

func createSession(c *tmux.Client, paths platform.Paths, version tmux.Version) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find own binary: %w", err)
	}
	colorterm := os.Getenv("COLORTERM")
	conf := tmux.Conf{
		Version:         version,
		DefaultTerminal: defaultTerminal(),
		OuterTerm:       os.Getenv("TERM"),
		TrueColor:       colorterm == "truecolor" || colorterm == "24bit",
		OverrideFile:    filepath.Join(paths.ConfigDir, "tmux.conf"),
	}
	confPath := filepath.Join(paths.DataDir, "tmux.conf")
	if err := conf.Write(confPath); err != nil {
		return fmt.Errorf("write tmux config: %w", err)
	}
	return c.NewSession(confPath, sessionName, tracksWindow, shellx.QuoteIfNeeded(self)+" --new-app tracks-window")
}

// defaultTerminal prefers tmux-256color, whose terminfo entry is
// missing on some systems (older macOS among them).
func defaultTerminal() string {
	if exec.Command("infocmp", "tmux-256color").Run() == nil {
		return "tmux-256color"
	}
	return "screen-256color"
}
