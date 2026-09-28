package agents

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/shellx"
)

// The pane scaffolding and quoting are v1's (internal/agent), without
// the exit sentinel, which comes with supervision.

// Wrapper is the shell scaffolding tracks puts around an agent command
// inside a tmux pane: the environment every track exports, and the
// login shell the pane falls back to so it stays usable afterwards.
//
// Shared rather than duplicated per provider because a difference here
// is not a wording difference — it is a track that never reports
// finishing, or a helper script that can't find its daemon.
type Wrapper struct {
	// TrackID is exported as TRACKS_ID so in-worktree helper scripts
	// can identify which track is calling. Every process in the pane
	// inherits it, whichever agent binary runs.
	TrackID string

	// SocketDir is exported as TRACKS_SOCKET_DIR so those same scripts
	// can find the daemon.
	SocketDir string

	// BinDir, when set, is prepended to PATH so a bare `tracks`
	// resolves even when the binary isn't on the login-shell PATH.
	BinDir string
}

// Command wraps an already-assembled agent command line — flags
// quoted, joined, provider's business — in that scaffolding, and
// returns the single string tmux should run in the new pane.
//
// tmux's `new-window <command>` hands its argument to /bin/sh -c, so
// this must produce a string rather than an argv.
func (w Wrapper) Command(line CommandLine) string {
	inner := string(line)
	// The pane stays alive as a login shell once the agent exits, so
	// the user can keep working in the worktree.
	inner += "\nexec ${SHELL:-bash} -l"

	env := "TRACKS_ID=" + shellx.Quote(w.TrackID) +
		" TRACKS_SOCKET_DIR=" + shellx.Quote(w.SocketDir)
	if w.BinDir != "" {
		// $PATH is expanded by the outer shell that runs this line.
		env += " PATH=" + shellx.Quote(w.BinDir) + `:"$PATH"`
	}

	// sh, not bash, for the outer wrapper: /bin/sh is the only shell
	// tmux relies on. The user's $SHELL is invoked only at the fallback
	// step above.
	return env + " sh -c " + shellx.Quote(inner)
}

// CommandLine is an agent command whose values have been quoted for
// /bin/sh. The type exists so Wrapper.Command cannot be handed a
// string that was assembled by concatenation — with two providers
// building their own flags, "remember to quote" is not a guarantee.
//
// Build one with Line.
type CommandLine string

// Line assembles an agent command: the binary, bare flags, and quoted
// values. Flags are emitted verbatim because they are literals in the
// source; everything that came from config, a track, or a user is
// quoted.
type Line struct{ parts []string }

// NewLine starts a command line for the given binary.
func NewLine(binary string) *Line {
	return &Line{parts: []string{shellx.Quote(binary)}}
}

// Arg appends a positional argument.
func (l *Line) Arg(v string) *Line {
	l.parts = append(l.parts, shellx.Quote(v))
	return l
}

// Flag appends a valueless flag, e.g. --force.
func (l *Line) Flag(name string) *Line {
	l.parts = append(l.parts, name)
	return l
}

// Set appends a flag and its value, e.g. --model gpt-5.3-codex.
func (l *Line) Set(name, value string) *Line {
	l.parts = append(l.parts, name, shellx.Quote(value))
	return l
}

// SetIf appends a flag and its value only when the value is non-empty.
// Every provider has flags that are omitted rather than passed empty —
// an empty --model would select a model named "".
func (l *Line) SetIf(name, value string) *Line {
	if value == "" {
		return l
	}
	return l.Set(name, value)
}

// Build returns the assembled command line.
func (l *Line) Build() CommandLine { return CommandLine(strings.Join(l.parts, " ")) }
