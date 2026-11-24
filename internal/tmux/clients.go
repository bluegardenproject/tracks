package tmux

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// ClientOf is the client showing pane: of those attached to its
// session, the one used last.
func (c *Client) ClientOf(pane string) (string, error) {
	client, err := c.run("display-message", "-p", "-t", pane, "#{client_name}")
	if err == nil && client == "" {
		err = errors.New("no client shows the pane")
	}
	return client, err
}

// ShownWindows are the windows of session that its attached clients
// show.
func (c *Client) ShownWindows(session string) ([]string, error) {
	out, err := c.run("list-clients", "-t", "="+session, "-F", "#{window_id}")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// LastKey is when a client showing window last had a key or a click,
// the zero time when none shows it. tmux keeps it in whole seconds, and
// output to the client doesn't change it.
func (c *Client) LastKey(window string) (time.Time, error) {
	out, err := c.run("list-clients", "-F", "#{window_id}\t#{client_activity}")
	if err != nil {
		return time.Time{}, err
	}
	return lastKey(out, window), nil
}

// lastKey is the latest activity among list-clients' lines out of the
// clients showing window.
func lastKey(out, window string) time.Time {
	var last time.Time
	for _, line := range strings.Split(out, "\n") {
		w, at, ok := strings.Cut(line, "\t")
		if !ok || w != window {
			continue
		}
		if secs, err := strconv.ParseInt(at, 10, 64); err == nil && time.Unix(secs, 0).After(last) {
			last = time.Unix(secs, 0)
		}
	}
	return last
}

// SwitchClient shows target, a window, on client.
func (c *Client) SwitchClient(client, target string) error {
	_, err := c.run("switch-client", "-c", client, "-t", target)
	return err
}

// Tell shows msg in client's status line.
func (c *Client) Tell(client, msg string) error {
	_, err := c.run("display-message", "-c", client, "-l", msg)
	return err
}

// RunShell runs command in the background on the server, with the
// server's environment, and doesn't wait for it.
func (c *Client) RunShell(command string) error {
	_, err := c.run("run-shell", "-b", command)
	return err
}
