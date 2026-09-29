// Package rpc is how the CLI and the popups talk to the daemon: one
// JSON request per connection on a Unix socket, answered by progress
// lines and then one result or error line.
package rpc

import (
	"encoding/json"

	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// The methods.
const (
	Ping      = "ping"
	Shutdown  = "shutdown"
	Create    = "create"
	List      = "list"
	End       = "end"
	Resume    = "resume"
	Clean     = "clean"
	Archive   = "archive"
	Unarchive = "unarchive"
	Filter    = "filter"
	Report    = "report"
	Derail    = "derail"
	// Watch sends a progress line right away and one after each change
	// to the tracks, until the daemon exits.
	Watch = "watch"
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

// ListParams asks for Station's list: under its filter, when one is on.
type ListParams struct {
	Station bool `json:"station,omitempty"`
}

// ListResult are the open tracks, in window order, then the most
// recently ended ones; or for Station under a filter, the tracks it
// picks, and the filter.
type ListResult struct {
	Tracks []tracks.Listed `json:"tracks"`
	Filter track.Filter    `json:"filter"`
}

// FilterParams sets Station's filter to Set, the zero Filter clearing
// it; without Set, it's only read. The result is a FilterResult.
type FilterParams struct {
	Set *track.Filter `json:"set,omitempty"`
}

// FilterResult is Station's filter.
type FilterResult struct {
	Filter track.Filter `json:"filter"`
}

// UnarchiveParams names the archived track to put back in Station.
type UnarchiveParams struct {
	ID string `json:"id"`
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

// ArchiveParams names the ended track to archive. Force removes its
// worktrees even with unsaved work in them; the result is a CleanResult.
type ArchiveParams struct {
	ID    string `json:"id"`
	Force bool   `json:"force,omitempty"`
}

// DerailParams names the ended track to delete for good. Check only
// looks for work that would be lost; Force deletes it even with some.
// The result is a CleanResult.
type DerailParams struct {
	ID    string `json:"id"`
	Check bool   `json:"check,omitempty"`
	Force bool   `json:"force,omitempty"`
}

// CleanResult is the unsaved work Clean found, one line per worktree,
// such as "web: 3 changed files". Nothing was removed when there's some.
type CleanResult struct {
	Unsaved []string `json:"unsaved,omitempty"`
}
