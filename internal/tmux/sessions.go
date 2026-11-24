package tmux

import (
	"fmt"
	"strings"
)

// NewKeptSession starts a detached session called name, its one window
// running command in dir, and returns its pane. The pane stays when the
// command exits (remain-on-exit), set in the same call, so a command
// that exits at once still leaves it.
func (c *Client) NewKeptSession(name, window, dir, command string) (string, error) {
	args := []string{"new-session", "-d", "-s", name, "-n", window, "-P", "-F", "#{pane_id}"}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, command)
	}
	args = append(args, ";", "set-option", "-w", "-t", "="+name+":", "remain-on-exit", "on")
	return c.run(args...)
}

// KillSession ends session and what runs in it.
func (c *Client) KillSession(session string) error {
	_, err := c.run("kill-session", "-t", "="+session)
	return err
}

// SetSessionOption sets an option of session, such as status-format[0].
func (c *Client) SetSessionOption(session, name, value string) error {
	_, err := c.run("set-option", "-t", "="+session+":", name, value)
	return err
}

// ListAllPanes returns the panes of every session.
func (c *Client) ListAllPanes() ([]Pane, error) {
	return c.listPanes("list-panes", "-a")
}

// SessionClients are the clients attached to session.
func (c *Client) SessionClients(session string) ([]string, error) {
	out, err := c.run("list-clients", "-t", "="+session, "-F", "#{client_name}")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// RefreshStatus redraws client's status line now, rather than at the
// next status-interval.
func (c *Client) RefreshStatus(client string) error {
	_, err := c.run("refresh-client", "-S", "-t", client)
	return err
}

// ClientSession is the session client is attached to. It's read from
// list-clients: display-message -c reports the session of the client
// used last, which may be another.
func (c *Client) ClientSession(client string) (string, error) {
	out, err := c.run("list-clients", "-F", "#{client_name}\t#{client_session}")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if name, session, ok := strings.Cut(line, "\t"); ok && name == client {
			return session, nil
		}
	}
	return "", fmt.Errorf("no client %s", client)
}

// DetachClient detaches client from its session.
func (c *Client) DetachClient(client string) error {
	_, err := c.run("detach-client", "-t", client)
	return err
}
