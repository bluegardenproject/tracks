// Package rpc is how the CLI and the popups talk to the daemon: one
// JSON request per connection on a Unix socket, answered by progress
// lines and then one result or error line.
package rpc

import (
	"encoding/json"

	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// The methods.
const (
	Ping     = "ping"
	Shutdown = "shutdown"
	Create   = "create"
	List     = "list"
	End      = "end"
)

// Request is one call.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Reply is one line of the answer: progress, or the last line with the
// result or the error.
type Reply struct {
	Progress string          `json:"progress,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	// Problem says Error is worded for the user.
	Problem bool `json:"problem,omitempty"`
}

// Last reports whether r ends the answer.
func (r Reply) Last() bool { return r.Progress == "" }

// PingResult says which daemon answered. Exe and ExeModified are its
// binary and that file's time when it started, so a client can tell a
// daemon running an older build of the same binary.
type PingResult struct {
	Version     string `json:"version"`
	PID         int    `json:"pid"`
	Exe         string `json:"exe,omitempty"`
	ExeModified int64  `json:"exe_modified,omitempty"` // Unix nanoseconds
}

// CreateParams is a new track, and the tmux client that asked, which
// hears how it went when the form is closed before.
type CreateParams struct {
	tracks.Request
	Client string `json:"client,omitempty"`
}

// CreateResult is the new track and its window.
type CreateResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Window string `json:"window"`
}

// ListResult are the open tracks, in window order.
type ListResult struct {
	Tracks []tracks.Listed `json:"tracks"`
}

// EndParams names the track to end.
type EndParams struct {
	ID string `json:"id"`
}
