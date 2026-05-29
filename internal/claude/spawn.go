// Package claude wraps how `tracks` invokes the `claude` CLI as a
// long-running interactive agent inside a tmux window.
//
// Claude runs *interactively* (no `-p` / one-shot mode) so the user
// can switch into the track's tmux window and keep chatting with
// the agent after it finishes the initial task. The tmux pane
// supplies the TTY; we just compose the right argv and hand it to
// tmux via `internal/tmux`.
package claude

import (
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// SpawnOptions describes everything needed to launch one Claude
// session inside a tmux pane.
type SpawnOptions struct {
	// CLIBinary is the claude executable. Either a bare name (PATH
	// lookup) or an absolute path.
	CLIBinary string

	// PermissionMode is passed as --permission-mode.
	PermissionMode string

	// TaskPrompt is the assembled prompt (user's text + the
	// hardcoded taskSuffix below). Passed as the positional
	// argument to claude so the TUI opens pre-filled.
	TaskPrompt string

	// AddDirs are the absolute paths passed as --add-dir for each
	// worktree the track owns.
	AddDirs []string

	// CWD is the directory claude is launched from. Conventionally
	// the first AddDir so paths in the assistant's output are
	// relative to the primary repo.
	CWD string

	// TrackID is exported as TRACKS_ID in the child's env so any
	// in-worktree helper script (e.g. tracks-add-repo) can identify
	// which track is calling.
	TrackID string

	// SocketDir is exported as TRACKS_SOCKET_DIR so the same helper
	// scripts can find the daemon.
	SocketDir string
}

// taskSuffix is appended to every task prompt the daemon sends to
// Claude. Hardcoded here so we can update the wording with the
// binary instead of asking every user to edit YAML.
//
// Two concerns only:
//
//  1. Make sure Claude knows it's running interactively. The
//     previous "When you finish, emit TRACKS_PR_URL" wording made
//     it treat every task as one-shot and exit after a single
//     response.
//  2. Surface the TRACKS_PR_URL marker contract so the dashboard
//     can detect PR creation — phrased as a side-channel, not a
//     finish signal.
//
// Repo / branch / commit conventions live in whatever CLAUDE.md
// the user has configured; Claude picks them up automatically.
// `tracks` deliberately does not duplicate or reference them
// here, so this binary stays useful for any project.
const taskSuffix = "" +
	"You're running interactively inside a `tracks` worktree (the " +
	"TRACKS_ID env var is set). The user can switch into this tmux " +
	"pane at any time to reply. Stay engaged: if the task naturally " +
	"ends with a question or a confirmation, ask it and wait — do " +
	"NOT wrap up the session just to acknowledge completion.\n\n" +
	"If you open a PR at any point, include the URL on its own line " +
	"as `TRACKS_PR_URL=<url>` so the tracks dashboard surfaces it."

// BuildOptions assembles SpawnOptions from a Track and Config.
// Returns an error when the configuration is incomplete (e.g. no
// worktrees on the track).
func BuildOptions(cfg config.Config, t state.Track, socketDir string) (SpawnOptions, error) {
	if len(t.Repos) == 0 {
		return SpawnOptions{}, errors.New("track has no repos")
	}
	addDirs := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		addDirs = append(addDirs, r.Path)
	}
	// Always-injected suffix below. Lives in code (not user
	// config) so we can iterate on the wording without users
	// having to edit YAML.
	prompt := strings.TrimRight(t.TaskPrompt, " \t\n\r") + "\n\n" + taskSuffix
	return SpawnOptions{
		CLIBinary:      cfg.Claude.Binary,
		PermissionMode: cfg.Claude.PermissionMode,
		TaskPrompt:     prompt,
		AddDirs:        addDirs,
		CWD:            t.Repos[0].Path,
		TrackID:        t.ID,
		SocketDir:      socketDir,
	}, nil
}

// ShellCommand returns the single shell-quoted command line tmux
// should run inside the new pane. tmux's `new-window <command>`
// passes its argument to /bin/sh -c so we must produce a string
// (not argv).
//
// Env vars (TRACKS_ID, TRACKS_SOCKET_DIR) are exported inline so
// they reach claude regardless of the parent shell's behavior.
func (o SpawnOptions) ShellCommand() string {
	parts := []string{}
	parts = append(parts,
		"TRACKS_ID="+shellQuote(o.TrackID),
		"TRACKS_SOCKET_DIR="+shellQuote(o.SocketDir),
	)
	parts = append(parts, shellQuote(o.CLIBinary))
	if o.TaskPrompt != "" {
		// Claude takes the prompt as a positional arg: it opens
		// the TUI pre-filled with that prompt.
		parts = append(parts, shellQuote(o.TaskPrompt))
	}
	if o.PermissionMode != "" {
		parts = append(parts, "--permission-mode", shellQuote(o.PermissionMode))
	}
	for _, d := range o.AddDirs {
		parts = append(parts, "--add-dir", shellQuote(d))
	}
	return strings.Join(parts, " ")
}

// shellQuote returns s wrapped in single quotes with embedded
// single quotes escaped. Safe for inclusion in any /bin/sh command
// line.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
