package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// Split is the direction of a new pane relative to the one it splits.
type Split int

const (
	// Right puts the new pane to the right, full height.
	Right Split = iota
	// Below puts the new pane underneath, full width.
	Below
)

// Pane is one pane of a window, with its geometry in cells.
type Pane struct {
	ID string
	// Title and Role are the @tracks_title and @tracks_role pane
	// options. Tracks doesn't use tmux's pane title, which programs in
	// the pane overwrite.
	Title, Role              string
	Left, Top, Width, Height int
}

// NewWindow adds a window to session without selecting it. Its first
// pane runs command in dir. It returns the window and pane IDs.
func (c *Client) NewWindow(session, name, dir, command string) (window, pane string, err error) {
	args := []string{"new-window", "-d", "-t", "=" + session + ":", "-n", name, "-P", "-F", "#{window_id} #{pane_id}"}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, command)
	}
	out, err := c.run(args...)
	if err != nil {
		return "", "", err
	}
	window, pane, ok := strings.Cut(out, " ")
	if !ok {
		return "", "", fmt.Errorf("tmux new-window: unexpected output %q", out)
	}
	return window, pane, nil
}

// SplitPane splits target and runs command in dir in the new pane,
// without selecting it. percent is the new pane's share of target's
// width or height; 0 means half.
func (c *Client) SplitPane(target string, dir Split, percent int, cwd, command string) (string, error) {
	args := []string{"split-window", "-d", "-t", target, "-P", "-F", "#{pane_id}"}
	if dir == Right {
		args = append(args, "-h", "-f")
	} else {
		args = append(args, "-v")
	}
	if percent > 0 {
		args = append(args, "-l", strconv.Itoa(percent)+"%")
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if command != "" {
		args = append(args, command)
	}
	return c.run(args...)
}

// ListPanes returns the panes of window.
func (c *Client) ListPanes(window string) ([]Pane, error) {
	out, err := c.run("list-panes", "-t", window, "-F",
		"#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}\t#{@tracks_role}\t#{@tracks_title}")
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 7)
		if len(f) != 7 {
			continue
		}
		n := make([]int, 4)
		for i := range n {
			n[i], _ = strconv.Atoi(f[i+1])
		}
		panes = append(panes, Pane{ID: f[0], Left: n[0], Top: n[1], Width: n[2], Height: n[3], Role: f[5], Title: f[6]})
	}
	return panes, nil
}

// SourceFile loads a config file into the running server.
func (c *Client) SourceFile(path string) error {
	_, err := c.run("source-file", path)
	return err
}

// DisplayMessage shows msg in the status line of the server's clients.
func (c *Client) DisplayMessage(msg string) error {
	_, err := c.run("display-message", "-l", msg)
	return err
}

// SelectPane makes pane the active pane of its window.
func (c *Client) SelectPane(pane string) error {
	_, err := c.run("select-pane", "-t", pane)
	return err
}

// ResizePaneHeight sets pane's height in rows.
func (c *Client) ResizePaneHeight(pane string, rows int) error {
	_, err := c.run("resize-pane", "-t", pane, "-y", strconv.Itoa(rows))
	return err
}

// SetPaneOption sets a pane option, such as @tracks_role.
func (c *Client) SetPaneOption(pane, name, value string) error {
	_, err := c.run("set-option", "-p", "-t", pane, name, value)
	return err
}

// SetWindowOption sets a window option.
func (c *Client) SetWindowOption(window, name, value string) error {
	_, err := c.run("set-option", "-w", "-t", window, name, value)
	return err
}

// WindowOption returns a window option's value, "" when unset.
func (c *Client) WindowOption(window, name string) (string, error) {
	return c.run("show-options", "-w", "-v", "-q", "-t", window, name)
}
