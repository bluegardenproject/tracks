package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

// start opens Tracks: it starts the tmux server when needed and attaches
// this terminal to it.
func start(version string) error {
	tmuxVersion, err := tmux.InstalledVersion()
	if err != nil {
		return err
	}
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	c := tmux.New(paths.TmuxSocket)

	action := planStartup(tmux.LocationOf(os.Getenv("TMUX"), paths.TmuxSocket), c.HasSession(sessionName))
	switch action {
	case startRefuse:
		return errInsideTmux
	case startCreate:
		if err := createSession(c, paths, tmuxVersion, version); err != nil {
			return err
		}
	}
	// Without a daemon Tracks still opens; creating a track says why.
	if _, err := ensureDaemon(context.Background(), c, paths, version); err != nil {
		fmt.Fprintln(os.Stderr, "tracks:", err)
	}
	if action == startSelect {
		return c.SelectWindow(sessionName + ":0")
	}
	return c.Attach(sessionName)
}

func createSession(c *tmux.Client, paths platform.Paths, tmuxVersion tmux.Version, version string) error {
	command, err := selfCommand()
	if err != nil {
		return err
	}

	t, _ := loadTheme(paths)
	if err := writeThemeConf(paths, t, version, command); err != nil {
		return err
	}
	colorterm := os.Getenv("COLORTERM")
	conf := tmux.Conf{
		Version:         tmuxVersion,
		DefaultTerminal: defaultTerminal(),
		OuterTerm:       os.Getenv("TERM"),
		TrueColor:       colorterm == "truecolor" || colorterm == "24bit",
		ThemeFile:       themeConfPath(paths),
		OverrideFile:    filepath.Join(paths.ConfigDir, "tmux.conf"),
		Command:         command,
	}
	confPath := filepath.Join(paths.DataDir, "tmux.conf")
	if err := conf.Write(confPath); err != nil {
		return fmt.Errorf("write tmux config: %w", err)
	}
	if err := c.NewSession(confPath, sessionName, tracksWindow, command+" tracks-window"); err != nil {
		return err
	}
	return c.SetWindowOption(sessionName+":0", "pane-border-status", "off")
}

// defaultTerminal prefers tmux-256color, whose terminfo entry is
// missing on some systems (older macOS among them).
func defaultTerminal() string {
	if exec.Command("infocmp", "tmux-256color").Run() == nil {
		return "tmux-256color"
	}
	return "screen-256color"
}
