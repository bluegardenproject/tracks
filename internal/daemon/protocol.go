// Package daemon implements the long-running `tracks` background
// process and the over-socket protocol the CLI uses to talk to it.
//
// One daemon per tmux session. The CLI subcommands (`tracks new`,
// `tracks ls`, …) are thin wrappers that dial the daemon's socket
// and exchange newline-delimited JSON messages.
package daemon

import (
	"encoding/json"

	"github.com/bluegardenproject/tracks/internal/state"
)

// Method is the request kind.
type Method string

const (
	// MethodPing checks daemon liveness and returns its version + pid.
	MethodPing Method = "ping"

	// MethodLs returns every known track.
	MethodLs Method = "ls"

	// MethodGet returns one track by ID.
	MethodGet Method = "get"

	// MethodNew creates a new track: provisions worktrees, registers
	// state, and (in v1+) spawns Claude. The handler that actually
	// spawns Claude is added in step 7; this step accepts the
	// request and writes the state row.
	MethodNew Method = "new"

	// MethodDone marks a track as done: SIGTERMs Claude if alive,
	// removes worktrees, retains branches.
	MethodDone Method = "done"

	// MethodKill is MethodDone with prejudice — SIGKILL immediately.
	MethodKill Method = "kill"

	// MethodAddRepo is called by Claude (from inside a worktree) to
	// add another repo's worktree to a running track.
	MethodAddRepo Method = "add_repo"

	// MethodPendingPrompts lists outstanding permission prompts the
	// daemon is holding open on behalf of Claude.
	MethodPendingPrompts Method = "pending_prompts"

	// MethodAnswerPrompt answers a pending prompt with allow/deny.
	MethodAnswerPrompt Method = "answer_prompt"

	// MethodShutdown asks the daemon to exit. Used when the tmux
	// session is being torn down explicitly.
	MethodShutdown Method = "shutdown"

	// MethodForget removes a single track entry from persistent
	// state. The track must already be in a terminal status
	// (Done / Errored) — forgetting a running track would orphan
	// the supervisor goroutine.
	MethodForget Method = "forget"

	// MethodPruneCompleted removes every track entry with a
	// terminal status. Returns the count removed.
	MethodPruneCompleted Method = "prune_completed"
)

// Request is the wire payload from CLI → daemon.
type Request struct {
	Method Method          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the wire payload from daemon → CLI.
//
// On success Ok=true and Result holds the method-specific payload.
// On failure Ok=false and Error is a human-readable message.
type Response struct {
	Ok     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// PingResult is returned by MethodPing.
type PingResult struct {
	Version string `json:"version"`
	PID     int    `json:"pid"`
}

// LsResult is returned by MethodLs.
type LsResult struct {
	Tracks []state.Track `json:"tracks"`
}

// GetParams is the payload for MethodGet.
type GetParams struct {
	ID string `json:"id"`
}

// GetResult is the payload for MethodGet.
type GetResult struct {
	Track state.Track `json:"track"`
	Found bool        `json:"found"`
}

// NewParams is the payload for MethodNew.
type NewParams struct {
	// Repos is the list of repo names from config the user picked.
	Repos []string `json:"repos"`
	// TaskPrompt is the free-form prompt typed by the user. The
	// daemon creates a placeholder branch (tracks/<id-tail>) and
	// Claude is expected to rename it to a proper <type>/<slug>
	// before its first commit, per the user's CLAUDE.md rules.
	TaskPrompt string `json:"task_prompt"`
}

// NewResult is the payload for MethodNew.
type NewResult struct {
	TrackID string `json:"track_id"`
	Branch  string `json:"branch"`
}

// DoneParams / KillParams.
type DoneParams struct {
	ID string `json:"id"`
}

// AddRepoParams is the payload for MethodAddRepo.
type AddRepoParams struct {
	// TrackID identifies the calling track (passed via TRACKS_ID env
	// var when Claude is spawned, then forwarded by the in-worktree
	// helper script).
	TrackID string `json:"track_id"`
	// RepoName matches a config.Repo.Name.
	RepoName string `json:"repo_name"`
}

// AddRepoResult tells the caller where the new worktree lives.
type AddRepoResult struct {
	WorktreePath string `json:"worktree_path"`
}

// PendingPrompt describes one outstanding permission request.
type PendingPrompt struct {
	ID      string `json:"id"`
	TrackID string `json:"track_id"`
	Tool    string `json:"tool"`
	Detail  string `json:"detail"`
}

// PendingPromptsResult is returned by MethodPendingPrompts.
type PendingPromptsResult struct {
	Prompts []PendingPrompt `json:"prompts"`
}

// AnswerPromptParams answers one pending prompt.
type AnswerPromptParams struct {
	ID    string `json:"id"`
	Allow bool   `json:"allow"`
}

// ForgetParams selects which track to drop from state.
type ForgetParams struct {
	ID string `json:"id"`
}

// PruneCompletedResult reports how many entries were removed by
// MethodPruneCompleted.
type PruneCompletedResult struct {
	Removed int `json:"removed"`
}
