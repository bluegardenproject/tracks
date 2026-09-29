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
	Resume   = "resume"
	Clean    = "clean"
	Report   = "report"
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

// CreateResult is the new or resumed track and its window.
type CreateResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Window string `json:"window"`
}

// ListResult are the open tracks, in window order, then the most
// recently ended ones.
type ListResult struct {
	Tracks []tracks.Listed `json:"tracks"`
}

// EndParams names the track to end.
type EndParams struct {
	ID string `json:"id"`
}

// ReportParams says event happened to the track, one of track.Event's
// values, and that its agent opened the pull requests at PRs. Either
// may be empty.
type ReportParams struct {
	ID    string   `json:"id"`
	Event string   `json:"event,omitempty"`
	PRs   []string `json:"prs,omitempty"`
}

// ResumeParams names the track to resume. Recreate re-creates its
// worktrees that are gone.
type ResumeParams struct {
	ID       string `json:"id"`
	Recreate bool   `json:"recreate,omitempty"`
}

// ResumeResult is the resumed track, or the worktrees that couldn't be
// found, one line per repo such as "web: /path/to/worktree". Nothing
// was done when there are some.
type ResumeResult struct {
	CreateResult
	Missing []string `json:"missing,omitempty"`
}

// CleanParams names the track to clean. Check only looks for unsaved
// work; Force removes the worktrees even with some in them.
type CleanParams struct {
	ID    string `json:"id"`
	Check bool   `json:"check,omitempty"`
	Force bool   `json:"force,omitempty"`
}

// CleanResult is the unsaved work Clean found, one line per worktree,
// such as "web: 3 changed files". Nothing was removed when there's some.
type CleanResult struct {
	Unsaved []string `json:"unsaved,omitempty"`
}
