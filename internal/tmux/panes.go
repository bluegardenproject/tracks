package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// Split is the direction of a new pane relative to the one it splits.
type Split int

const (
	// Right puts the new pane to the right of the target, as tall as
	// it: a split of the target, not of the whole window, so a pane
	// left of it, as the Platform, keeps its width.
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
	Title, Role string
	// State is the @tracks_state pane option, which a pane's own
	// script sets: a setup pane's "running" or "failed <code>".
	State string
	// Track is the @tracks_track pane option: the track a pane outside
	// its track's window belongs to. Window and Session are the pane's.
	Track, Window, Session string
	// Dead says what the pane ran has exited, and the pane stays
	// (remain-on-exit).
	Dead bool
	// Key and Port are a dev-server pane's @tracks_key, which names its
	// repo and server, and @tracks_port, 0 for none.
	Key                      string
	Port                     int
	PID                      int // the pane's first process, which leads its process group
	Left, Top, Width, Height int
	TTY                      string // the terminal the pane's programs write to
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
		args = append(args, "-h")
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
	return c.listPanes("list-panes", "-t", window)
}

// ListSessionPanes returns the panes of every window of session.
func (c *Client) ListSessionPanes(session string) ([]Pane, error) {
	return c.listPanes("list-panes", "-s", "-t", "="+session)
}

// paneFormat is what listPanes asks tmux for; the title goes last, as
// the one field a program can fill with anything.
const paneFormat = "#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}\t#{pane_pid}\t" +
	"#{@tracks_port}\t#{pane_tty}\t#{@tracks_role}\t#{@tracks_state}\t#{@tracks_key}\t#{@tracks_track}\t" +
	"#{window_id}\t#{session_name}\t#{pane_dead}\t#{@tracks_title}"

func (c *Client) listPanes(args ...string) ([]Pane, error) {
	out, err := c.run(append(args, "-F", paneFormat)...)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 16)
		if len(f) == 15 {
			// The output's trailing whitespace is trimmed, taking the
			// last pane's tab before an empty title with it.
			f = append(f, "")
		}
		if len(f) != 16 {
			continue
		}
		n := make([]int, 6)
		for i := range n {
			n[i], _ = strconv.Atoi(f[i+1])
		}
		panes = append(panes, Pane{ID: f[0], Left: n[0], Top: n[1], Width: n[2], Height: n[3], PID: n[4], Port: n[5],
			TTY: f[7], Role: f[8], State: f[9], Key: f[10], Track: f[11], Window: f[12], Session: f[13], Dead: f[14] == "1",
			Title: f[15]})
	}
	return panes, nil
}

// SplitLeft opens a pane cols wide on the left of target's window, full
// height, running command in cwd, without selecting it.
func (c *Client) SplitLeft(target string, cols int, cwd, command string) (string, error) {
	args := []string{"split-window", "-d", "-h", "-b", "-f", "-l", strconv.Itoa(cols), "-t", target, "-P", "-F", "#{pane_id}"}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if command != "" {
		args = append(args, command)
	}
	return c.run(args...)
}

// CaptureHistory returns pane's visible rows and up to lines rows of
// scrollback above them, with wrapped lines joined.
func (c *Client) CaptureHistory(pane string, lines int) (string, error) {
	return c.run("capture-pane", "-p", "-J", "-S", "-"+strconv.Itoa(lines), "-t", pane)
}

// CapturePane returns what pane shows, its visible rows only.
func (c *Client) CapturePane(pane string) (string, error) {
	return c.run("capture-pane", "-p", "-t", pane)
}

// CapturePaneStyled is what pane shows, with the escapes that style it.
func (c *Client) CapturePaneStyled(pane string) (string, error) {
	return c.run("capture-pane", "-p", "-e", "-t", pane)
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

// RespawnPane kills what pane runs and starts command in dir there.
func (c *Client) RespawnPane(pane, dir, command string) error {
	args := []string{"respawn-pane", "-k", "-t", pane}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, command)
	}
	_, err := c.run(args...)
	return err
}

// SelectPane makes pane the active pane of its window.
func (c *Client) SelectPane(pane string) error {
	_, err := c.run("select-pane", "-t", pane)
	return err
}

// SendText types text into pane, as keys and not as a paste.
func (c *Client) SendText(pane, text string) error {
	_, err := c.run("send-keys", "-l", "-t", pane, "--", text)
	return err
}

// InMode reports whether pane is in a mode, such as copy mode while
// the user scrolls back: keys sent to it go to the mode, not to what
// runs there.
func (c *Client) InMode(pane string) (bool, error) {
	out, err := c.run("display-message", "-p", "-t", pane, "#{pane_in_mode}")
	return out == "1", err
}

// CancelMode takes pane out of its mode, such as copy mode, as the
// user's q would.
func (c *Client) CancelMode(pane string) error {
	_, err := c.run("send-keys", "-X", "-t", pane, "cancel")
	return err
}

// PasteText pastes text into pane at once, as one bracketed paste when
// the program there asked for them: keys typed meanwhile can't land
// inside it. A buffer of its own carries it and goes after.
func (c *Client) PasteText(pane, text string) error {
	_, err := c.run("set-buffer", "-b", "tracks-paste", "--", text, ";",
		"paste-buffer", "-p", "-d", "-b", "tracks-paste", "-t", pane)
	return err
}

// SendKey presses key, such as Enter, in pane.
func (c *Client) SendKey(pane, key string) error {
	_, err := c.run("send-keys", "-t", pane, key)
	return err
}

// ResizePaneWidth sets pane's width in columns.
func (c *Client) ResizePaneWidth(pane string, cols int) error {
	_, err := c.run("resize-pane", "-t", pane, "-x", strconv.Itoa(cols))
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

// Popup is a popup over a client, sized in cells or percentages
// ("80%"), running Command.
type Popup struct {
	Client, Width, Height, Command string
	// Background replaces the terminal's own background in the popup.
	Background string
	// Border draws a rounded border in that colour; "" draws none, for
	// a program that draws its own.
	Border string
}

// Popup opens p and returns once it closes.
func (c *Client) Popup(p Popup) error {
	_, err := c.run(popupArgs(p)...)
	return err
}

func popupArgs(p Popup) []string {
	args := []string{"display-popup", "-E", "-c", p.Client, "-w", p.Width, "-h", p.Height}
	if p.Border == "" {
		args = append(args, "-B")
	} else {
		args = append(args, "-b", "rounded", "-S", "fg="+p.Border)
	}
	if p.Background != "" {
		args = append(args, "-s", "bg="+p.Background)
	}
	return append(args, p.Command)
}

// ClientSize is client's terminal size in cells.
func (c *Client) ClientSize(client string) (width, height int, err error) {
	out, err := c.run("display-message", "-p", "-c", client, "#{client_width} #{client_height}")
	if err != nil {
		return 0, 0, err
	}
	if _, err := fmt.Sscan(out, &width, &height); err != nil {
		return 0, 0, fmt.Errorf("tmux client size %q: %w", out, err)
	}
	return width, height, nil
}
