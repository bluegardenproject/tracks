package tmux

import (
	"path/filepath"
	"strings"
)

// Location says whether a process runs inside tmux, and which server.
type Location int

const (
	// Outside is a plain terminal.
	Outside Location = iota
	// OwnServer is a pane of the server on the given socket.
	OwnServer
	// OtherServer is a pane of any other tmux server.
	OtherServer
)

// LocationOf reads a $TMUX value ("<socket path>,<pid>,<session>")
// relative to the server on socket.
func LocationOf(tmuxEnv, socket string) Location {
	if tmuxEnv == "" {
		return Outside
	}
	path, _, _ := strings.Cut(tmuxEnv, ",")
	if filepath.Base(path) == socket {
		return OwnServer
	}
	return OtherServer
}
