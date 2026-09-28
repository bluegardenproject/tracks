package tmux

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
