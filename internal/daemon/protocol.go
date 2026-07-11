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

	// MethodPromote turns a worktree-less ask/plan track into a work
	// track: it creates a branch + worktree off base and re-spawns
	// Claude in it with edit permissions.
	MethodPromote Method = "promote"

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

	// MethodServiceUp starts a named dev-server service for a track (and
	// any of its declared dependencies), waits for readiness, opens a
	// log-viewer pane in the track's tmux window, and fires a
	// ready-for-testing notification.
	MethodServiceUp Method = "service_up"

	// MethodServiceDown stops a single running service for a track,
	// running its pre_stop hooks first, and closes the viewer pane.
	MethodServiceDown Method = "service_down"

	// MethodServices returns the current service states and allocated
	// ports for a track.
	MethodServices Method = "services"

	// MethodProxySwitch sets the active upstream for a service's stable-port
	// proxy to a specific track's service, or clears it.
	MethodProxySwitch Method = "proxy_switch"

	// MethodProxyStatus returns the current state of all registered proxies.
	MethodProxyStatus Method = "proxy_status"

	// MethodResume re-opens a finished (done/errored) track's Claude session
	// by re-creating any removed worktrees and spawning claude --resume
	// <session-id>. The track's status moves back to running.
	MethodResume Method = "resume"
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

// Progress is sent zero or more times BEFORE the final Response on
// the same connection. The client distinguishes a Progress payload
// from a Response by the presence of the "progress" field. Used by
// long-running methods like MethodNew so the caller can show a
// live log while git fetches and worktree checkouts happen.
type Progress struct {
	Progress string `json:"progress"`
}

// PingResult is returned by MethodPing.
type PingResult struct {
	Version string `json:"version"`
	PID     int    `json:"pid"`
	// ExePath and ExeModUnixNano describe the on-disk binary the
	// daemon is running, captured at startup. The CLI uses them to
	// detect a stale daemon after a local rebuild: plain `go build`
	// (and `make build` on an unchanged commit) leave Version at its
	// hardcoded default, so a version match can't tell a fresh binary
	// from an old one — but the file's mtime always changes on
	// rebuild. Empty/zero when os.Executable() couldn't be resolved.
	ExePath        string `json:"exe_path,omitempty"`
	ExeModUnixNano int64  `json:"exe_mod_unix_nano,omitempty"`
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
	// Slug is an optional human label for the track. Independent
	// of the branch name. Shown in the dashboard. Empty allowed.
	Slug string `json:"slug,omitempty"`
	// ReviewRef, when set, makes this a review track: instead of
	// branching a fresh placeholder off base, the daemon checks the
	// given target out on a detached HEAD so there's something to
	// review. Accepts a GitHub PR URL (…/pull/123) or a branch name
	// (local or on origin). Only meaningful with a single repo.
	ReviewRef string `json:"review_ref,omitempty"`
	// Kind is the track type (work/review/ask/plan). Empty defaults to
	// work. ask/plan are worktree-less: the daemon points Claude at the
	// primary checkout read-only instead of creating a worktree.
	Kind string `json:"kind,omitempty"`
}

// NewResult is the payload for MethodNew.
type NewResult struct {
	TrackID string `json:"track_id"`
	Branch  string `json:"branch"`
	// WindowName is the tmux window the daemon opened for the track.
	// Returned so the CLI can switch to it without re-deriving the
	// name (the caller only has the track ID, not the full Track).
	WindowName string `json:"window_name"`
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

// PromoteParams is the payload for MethodPromote.
type PromoteParams struct {
	// ID is the worktree-less (ask/plan) track to promote to a work track.
	ID string `json:"id"`
}

// PromoteResult reports the branch and tmux window of the re-spawned
// work track.
type PromoteResult struct {
	Branch     string `json:"branch"`
	WindowName string `json:"window_name"`
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

// ServiceUpParams is the payload for MethodServiceUp.
type ServiceUpParams struct {
	TrackID     string `json:"track_id"`
	ServiceName string `json:"service_name"`
}

// ServiceUpResult is returned by MethodServiceUp.
type ServiceUpResult struct {
	Port    int    `json:"port"`
	LogPath string `json:"log_path"`
}

// ServiceDownParams is the payload for MethodServiceDown.
type ServiceDownParams struct {
	TrackID     string `json:"track_id"`
	ServiceName string `json:"service_name"`
}

// ServicesParams is the payload for MethodServices.
type ServicesParams struct {
	TrackID string `json:"track_id"`
}

// ServicesResult is returned by MethodServices.
type ServicesResult struct {
	Services []state.ServiceState `json:"services"`
	Ports    map[string]int       `json:"ports"`
}

// ProxySwitchParams is the payload for MethodProxySwitch.
// Set TrackID to activate that track's service as the upstream; leave it
// empty (or set it to "off") to clear the proxy (returns 503).
type ProxySwitchParams struct {
	ServiceName string `json:"service_name"`
	TrackID     string `json:"track_id"` // "" or "off" to clear
}

// ProxyStatusResult is returned by MethodProxyStatus.
type ProxyStatusResult struct {
	Proxies []ProxyEntryStatus `json:"proxies"`
}

// ResumeParams is the payload for MethodResume.
type ResumeParams struct {
	ID string `json:"id"`
}

// ResumeResult is returned by MethodResume.
type ResumeResult struct {
	// WindowName is the tmux window the daemon re-opened for the track.
	WindowName string `json:"window_name"`
}

// ProxyEntryStatus describes one proxy entry.
type ProxyEntryStatus struct {
	ServiceName string `json:"service_name"`
	PublicPort  int    `json:"public_port"`
	// Upstream is "host:port" when active, "" when inactive (503).
	Upstream string `json:"upstream"`
	// ActiveTrackID is the track whose service port is the current upstream,
	// derived by reverse-lookup against live track service ports.
	ActiveTrackID string `json:"active_track_id,omitempty"`
}
