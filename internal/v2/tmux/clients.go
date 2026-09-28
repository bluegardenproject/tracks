package tmux

import "errors"

// ClientOf is the client showing pane: of those attached to its
// session, the one used last.
func (c *Client) ClientOf(pane string) (string, error) {
	client, err := c.run("display-message", "-p", "-t", pane, "#{client_name}")
	if err == nil && client == "" {
		err = errors.New("no client shows the pane")
	}
	return client, err
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
