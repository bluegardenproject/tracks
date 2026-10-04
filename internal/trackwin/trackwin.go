// Package trackwin builds and changes the tmux window of a track: the
// agent pane on the left, and a column on the right with terminal and
// dev-server panes stacked.
package trackwin

import (
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// ColumnPercent is the right column's share of the window width.
const ColumnPercent = 30

// Pane roles, stored in the @tracks_role pane option.
const (
	RoleAgent     = "agent"
	RoleTerminal  = "terminal"
	RoleDevServer = "dev-server"
	RoleSetup     = "setup"
)

// StateOption is the pane option a pane's own script keeps its state
// in, read back as tmux.Pane.State.
const StateOption = "@tracks_state"

// Window options of a track's window. dirOption holds its working
// directory, so panes added later start there too.
const (
	trackOption = "@tracks_id"
	dirOption   = "@tracks_dir"
	kindOption  = "@tracks_kind"
	repoOption  = "@tracks_repo"
	// AttentionOption is set while the track needs the user; the
	// footer marks its slot.
	AttentionOption = "@tracks_attention"
)

// SetAttention marks window as needing the user, or not.
func SetAttention(t Tmux, window string, on bool) error {
	value := ""
	if on {
		value = "1"
	}
	return t.SetWindowOption(window, AttentionOption, value)
}

// Process is a command shown in a pane under a title.
type Process struct {
	Title   string
	Command string
}

// Spec describes a new track window.
type Spec struct {
	Track      string // the track's ID
	Name       string // window name
	Kind       string // work, ask, plan, review, doc
	Repo       string // the repos' names, comma-separated
	Dir        string // working directory of every pane
	Agent      Process
	Terminals  int // terminal panes opened with the window
	DevServers []Process
}

// Window identifies a track's window and its agent pane.
type Window struct {
	ID    string
	Agent string
}

// Tmux is what trackwin needs from a tmux server.
type Tmux interface {
	NewWindow(session, name, dir, command string) (window, pane string, err error)
	SplitPane(target string, dir tmux.Split, percent int, cwd, command string) (string, error)
	RespawnPane(pane, dir, command string) error
	ListPanes(window string) ([]tmux.Pane, error)
	SetPaneOption(pane, name, value string) error
	SelectPane(pane string) error
	ResizePaneHeight(pane string, rows int) error
	SetWindowOption(window, name, value string) error
	WindowOption(window, name string) (string, error)
	ListWindows(session string) ([]tmux.Window, error)
	SelectWindow(window string) error
}

// Open creates the window for s in session, without switching to it.
// The agent pane is active, so switching to the window lands there.
func Open(t Tmux, session string, s Spec) (Window, error) {
	id, agent, err := t.NewWindow(session, s.Name, s.Dir, s.Agent.Command)
	if err != nil {
		return Window{}, err
	}
	w := Window{ID: id, Agent: agent}
	if err := t.SetWindowOption(id, "automatic-rename", "off"); err != nil {
		return w, err
	}
	for name, value := range map[string]string{trackOption: s.Track, dirOption: s.Dir, kindOption: s.Kind, repoOption: s.Repo} {
		if err := t.SetWindowOption(id, name, value); err != nil {
			return w, err
		}
	}
	if err := label(t, agent, RoleAgent, s.Agent.Title); err != nil {
		return w, err
	}
	for range s.Terminals {
		if _, err := add(t, id, s.Dir, RoleTerminal, Process{Title: "terminal"}); err != nil {
			return w, err
		}
	}
	for _, dev := range s.DevServers {
		if _, err := add(t, id, s.Dir, RoleDevServer, dev); err != nil {
			return w, err
		}
	}
	return w, t.SelectPane(agent)
}

// Respawn restarts window's agent pane on s's agent, in s.Dir, and
// makes the window s's: its kind, repos and directory. The old agent's
// exit code is cleared; the other panes stay as they are. It's how a
// track restarts its agent, or changes kind, without a new window.
func Respawn(t Tmux, window string, s Spec) (Window, error) {
	panes, err := t.ListPanes(window)
	if err != nil {
		return Window{}, err
	}
	agent, _ := layout(panes)
	if agent == nil {
		return Window{}, fmt.Errorf("window %s has no agent pane", window)
	}
	w := Window{ID: window, Agent: agent.ID}
	if err := t.SetWindowOption(window, agents.ExitOption, ""); err != nil {
		return w, err
	}
	if err := t.RespawnPane(agent.ID, s.Dir, s.Agent.Command); err != nil {
		return w, err
	}
	for name, value := range map[string]string{dirOption: s.Dir, kindOption: s.Kind, repoOption: s.Repo} {
		if err := t.SetWindowOption(window, name, value); err != nil {
			return w, err
		}
	}
	if err := label(t, agent.ID, RoleAgent, s.Agent.Title); err != nil {
		return w, err
	}
	return w, t.SelectPane(agent.ID)
}

// ErrNotTrackWindow means the window isn't a track's, such as the
// Tracks window.
var ErrNotTrackWindow = errors.New("not a track window")

// AddTerminal adds a shell pane to the bottom of window's right column
// and returns its ID.
func AddTerminal(t Tmux, window string) (string, error) {
	dir, err := t.WindowOption(window, dirOption)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", ErrNotTrackWindow
	}
	pane, err := add(t, window, dir, RoleTerminal, Process{Title: "terminal"})
	if err != nil {
		return "", err
	}
	return pane, t.SelectPane(pane)
}

// AddSetup adds a pane running p, a repo's setup, to the bottom of
// window's right column, in dir. The agent pane keeps focus.
func AddSetup(t Tmux, window, dir string, p Process) (string, error) {
	return add(t, window, dir, RoleSetup, p)
}

// add puts a pane at the bottom of the right column, creating the
// column when it doesn't exist, and evens out the column's heights.
func add(t Tmux, window, dir, role string, p Process) (string, error) {
	panes, err := t.ListPanes(window)
	if err != nil {
		return "", err
	}
	agent, column := layout(panes)
	if agent == nil {
		return "", fmt.Errorf("window %s has no agent pane", window)
	}

	var pane string
	if len(column) == 0 {
		pane, err = t.SplitPane(agent.ID, tmux.Right, ColumnPercent, dir, p.Command)
	} else {
		pane, err = t.SplitPane(column[len(column)-1].ID, tmux.Below, 0, dir, p.Command)
	}
	if err != nil {
		return "", err
	}
	if err := label(t, pane, role, p.Title); err != nil {
		return pane, err
	}
	return pane, even(t, window)
}

func label(t Tmux, pane, role, title string) error {
	if err := t.SetPaneOption(pane, "@tracks_role", role); err != nil {
		return err
	}
	return t.SetPaneOption(pane, "@tracks_title", title)
}

// even gives every pane in the right column the same height.
func even(t Tmux, window string) error {
	panes, err := t.ListPanes(window)
	if err != nil {
		return err
	}
	_, column := layout(panes)
	heights := columnHeights(column)
	// The last pane takes whatever is left, so it's never resized.
	for i, p := range column[:max(0, len(column)-1)] {
		if err := t.ResizePaneHeight(p.ID, heights[i]); err != nil {
			return err
		}
	}
	return nil
}
