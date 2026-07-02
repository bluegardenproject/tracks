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
	"os"
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

	// SessionID pins Claude's session UUID via --session-id, so the
	// daemon can find this track's transcript (and thus its token
	// usage) at a known path. Empty means don't pin one.
	SessionID string

	// SentinelPath is the path to a file the shell wrapper touches
	// the instant Claude exits, so the supervisor can finalize the
	// track without depending on pid death. Empty means no shell
	// wrapper / no sentinel handling.
	SentinelPath string
}

// taskSuffix is appended to every task prompt the daemon sends to
// Claude. Hardcoded here so we can update the wording with the
// binary instead of asking every user to edit YAML.
//
// Three concerns:
//
//  1. Make sure Claude knows it's running interactively (don't
//     treat every task as one-shot).
//  2. Mandate a code-review gate before any push / PR — the
//     review uses whatever conventions the repo documents.
//  3. Surface the TRACKS_PR_URL marker contract so the dashboard
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
	"**Mandatory pre-push review.** Before you run `git push` or " +
	"open a pull request:\n" +
	"  1. Invoke the dedicated review subagent via the Task tool:\n" +
	"     `Task({ subagent_type: \"tracks-reviewer\", prompt: " +
	"\"Review my changes in this worktree before push.\" })`\n" +
	"     The subagent is auto-discovered from the user's global " +
	"Claude config — no setup needed inside the worktree.\n" +
	"  2. Read the findings. The subagent ends its report with one of:\n" +
	"     `REVIEW OUTCOME: pass` or `REVIEW OUTCOME: blocked`.\n" +
	"  3. If blocked, address every `block` finding and re-run the " +
	"subagent. Do not push with unresolved blocks.\n" +
	"  4. Acknowledge `warn` findings in the PR description and " +
	"include the final `REVIEW OUTCOME` line so the human reviewer " +
	"knows what was already vetted.\n\n" +
	"If you open a PR at any point, include the URL on its own line " +
	"as `TRACKS_PR_URL=<url>` so the tracks dashboard surfaces it.\n\n" +
	"**Dev-server services.** Before starting any dev server manually, " +
	"run `tracks services` — it lists every configured service with its " +
	"status, port, and log path. If the output is empty there are no " +
	"services configured and you can skip this section. Otherwise always " +
	"use the tracks commands below instead of running `pnpm dev`, " +
	"`npm start`, etc. directly — the supervisor handles port allocation, " +
	"dependency ordering, and the stable-port proxy the app points at.\n\n" +
	"`$TRACKS_ID` is already set in the environment; the `--track` flag " +
	"is never needed.\n" +
	"  - `tracks services` — status, port, and log path for every service\n" +
	"  - `tracks up <name>` — start a service (starts dependencies first)\n" +
	"  - `tracks down <name>` — stop a running service\n" +
	"  - `tracks url <name>` — print the URL (stable proxy + track port)\n\n" +
	"The log path shown by `tracks services` is a plain file; read it " +
	"with `tail -f <path>` or `cat <path>` to check server output without " +
	"switching panes.\n\n" +
	"**Jira sync** (only if your task prompt references a Jira-style " +
	"ticket like ABC-123 and the Atlassian MCP tools are available):\n" +
	"  1. At the start, use `Bash` to read `git config user.email`. " +
	"Resolve it with `lookupJiraAccountId`, then call `editJiraIssue` " +
	"to set that account as the ticket's assignee.\n" +
	"  2. If you will be modifying code (not a read-only review), " +
	"call `getTransitionsForJiraIssue` and `transitionJiraIssue` " +
	"to move the ticket to the closest match for \"In Progress\". " +
	"If the task is a read-only audit, SKIP the status change.\n" +
	"  3. When you open a PR, transition the ticket to the closest " +
	"match for \"In Review\" (or \"Code Review\" / \"Awaiting Review\"). " +
	"Do NOT add a comment with the PR URL — the PR is already linked " +
	"automatically and a comment would be duplicate noise.\n" +
	"  4. Any Atlassian-tool error is non-fatal — note it in your " +
	"reply and carry on with the actual work."

// readOnlySuffix is appended for worktree-less (ask/plan) tracks. They
// point at the user's PRIMARY checkout (the one their editor watches),
// so the prompt makes the read-only contract explicit as a second line
// of defence behind plan permission mode (which is a default, not a
// hard sandbox — see BuildOptions).
const readOnlySuffix = "" +
	"\n\n**This is a read-only track.** You are pointed at the user's " +
	"primary checkout — the working copy their editor uses — NOT a " +
	"throwaway worktree. Do not modify any files, create branches, or " +
	"run mutating commands; investigate and answer (or produce a plan) " +
	"only. When the user is ready to implement, the track can be " +
	"promoted to its own worktree with `tracks promote <id>`."

// BuildOptions assembles SpawnOptions from a Track and Config.
// Returns an error when the configuration is incomplete (e.g. no
// worktrees on the track).
func BuildOptions(cfg config.Config, t state.Track, socketDir, sentinelPath string) (SpawnOptions, error) {
	// Worktree-less ask/plan tracks may carry no repos at all (a
	// general question not tied to a repo). Work/review always have at
	// least one — the daemon enforces that before spawning.
	if len(t.Repos) == 0 && !t.Kind.Worktreeless() {
		return SpawnOptions{}, errors.New("track has no repos")
	}
	addDirs := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		addDirs = append(addDirs, r.Path)
	}

	prompt := strings.TrimRight(t.TaskPrompt, " \t\n\r")

	// Work/review tracks get the work suffix (review gate, PR marker,
	// Jira sync). Worktree-less ask/plan tracks deliberately don't:
	// that framing is wrong for a read-only question and only risks
	// degrading the answer. They run in plan permission mode instead,
	// and get a short read-only contract *only* when repos are
	// attached — i.e. Claude can see the user's primary checkout and
	// must be told not to edit it. A repo-less question is sent
	// verbatim. (Plan mode is a strong default, not a hard sandbox —
	// it's interactively switchable; promotion is the path to editing.)
	permMode := cfg.Claude.PermissionMode
	if t.Kind.Worktreeless() {
		permMode = "plan"
		if len(t.Repos) > 0 {
			prompt += readOnlySuffix
		}
	} else {
		prompt += "\n\n" + taskSuffix
	}

	// CWD is the first worktree; for a repo-less ask, fall back to the
	// user's home dir so tmux has a valid directory to open the pane in.
	cwd := ""
	if len(t.Repos) > 0 {
		cwd = t.Repos[0].Path
	} else if home, err := os.UserHomeDir(); err == nil {
		cwd = home
	}

	return SpawnOptions{
		CLIBinary:      cfg.Claude.Binary,
		PermissionMode: permMode,
		TaskPrompt:     prompt,
		AddDirs:        addDirs,
		CWD:            cwd,
		TrackID:        t.ID,
		SocketDir:      socketDir,
		SentinelPath:   sentinelPath,
		SessionID:      t.SessionID,
	}, nil
}

// ShellCommand returns the single shell-quoted command line tmux
// should run inside the new pane. tmux's `new-window <command>`
// passes its argument to /bin/sh -c so we must produce a string
// (not argv).
//
// Layout:
//
//	TRACKS_ID=… TRACKS_SOCKET_DIR=… exec sh -c '
//	    claude <args>
//	    touch SENTINEL    # if SentinelPath is set
//	    exec ${SHELL:-bash} -l    # leave the user a usable prompt
//	'
//
// The shell-fallback piece is what keeps the pane alive after
// Claude exits — without it, tmux would render a "[exited]" dead
// pane and the user couldn't poke around the worktree.
func (o SpawnOptions) ShellCommand() string {
	claudeArgv := []string{shellQuote(o.CLIBinary)}
	if o.TaskPrompt != "" {
		// Claude takes the prompt as a positional arg: it opens
		// the TUI pre-filled with that prompt.
		claudeArgv = append(claudeArgv, shellQuote(o.TaskPrompt))
	}
	if o.PermissionMode != "" {
		claudeArgv = append(claudeArgv, "--permission-mode", shellQuote(o.PermissionMode))
	}
	if o.SessionID != "" {
		claudeArgv = append(claudeArgv, "--session-id", shellQuote(o.SessionID))
	}
	for _, d := range o.AddDirs {
		claudeArgv = append(claudeArgv, "--add-dir", shellQuote(d))
	}
	claudeLine := strings.Join(claudeArgv, " ")

	// Build the inner shell script.
	inner := claudeLine
	if o.SentinelPath != "" {
		inner += "\ntouch " + shellQuote(o.SentinelPath)
	}
	inner += "\nexec ${SHELL:-bash} -l"

	envPrefix := "TRACKS_ID=" + shellQuote(o.TrackID) +
		" TRACKS_SOCKET_DIR=" + shellQuote(o.SocketDir)

	// Outer sh -c "..." wrapper. We deliberately use sh (not bash)
	// for the outer because /bin/sh is the only shell tmux relies
	// on; the user's $SHELL is invoked only at the fallback step.
	return envPrefix + " sh -c " + shellQuote(inner)
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
