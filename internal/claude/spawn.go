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

	// TaskPrompt is the assembled prompt (user's text +
	// DefaultPromptSuffix). Passed as the positional argument to
	// claude so the TUI opens pre-filled.
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
	prompt := t.TaskPrompt
	if cfg.Claude.DefaultPromptSuffix != "" {
		// Always force a blank-line separator so the suffix never
		// gets glued onto the end of the user's task ("task.When
		// you finish, ...") regardless of how the suffix is
		// authored in YAML.
		suffix := strings.TrimLeft(cfg.Claude.DefaultPromptSuffix, " \t\n\r")
		prompt = strings.TrimRight(prompt, " \t\n\r") + "\n\n" + suffix
	}
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
