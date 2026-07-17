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

	// SessionID pins Claude's session UUID. In a normal spawn it is
	// passed as --session-id; when Resume=true it is passed as the
	// argument to --resume instead (no separate --session-id flag).
	// Empty means don't pin one.
	SessionID string

	// Resume, when true, tells ShellCommand to pass --resume <SessionID>
	// instead of a positional prompt + --session-id, continuing the
	// existing conversation rather than starting a new one.
	Resume bool

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
// Key concerns:
//
//  1. Make sure Claude knows it's running interactively (don't
//     treat every task as one-shot).
//  2. Mandate a code-review gate before any push / PR — the
//     review uses whatever conventions the repo documents.
//  3. Surface the TRACKS_PR_URL marker contract so the dashboard
//     can detect PR creation — phrased as a side-channel, not a
//     finish signal.
//  4. Keep output terse and comment-free — these sessions are read
//     in a dashboard, so brevity and clean diffs are project-agnostic
//     wins that belong in the binary, not per-user CLAUDE.md.
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
	"**Dev-server services.** When the user asks you to start (or run, " +
	"boot, spin up) the dev server, do NOT run `pnpm dev` / `npm " +
	"start` / `pnpm install` yourself. Run `tracks up <name>` instead: " +
	"it opens a dedicated pane in this track and runs the configured " +
	"start steps there (dependency install first, then the server) so " +
	"the process is visible and does not block you. It returns " +
	"immediately; the install and boot continue in the pane.\n\n" +
	"`$TRACKS_ID` is already set in the environment; the `--track` flag " +
	"is never needed.\n" +
	"  - `tracks services` lists configured services with status, port, " +
	"and log path. Run this first to find the service name (if it " +
	"prints nothing, this repo has no dev server configured; tell the " +
	"user and stop).\n" +
	"  - `tracks up <name>` opens a pane and starts the service (its " +
	"depends_on services first)\n" +
	"  - `tracks down <name>` stops a running service\n" +
	"  - `tracks url <name>` prints the URL (stable proxy + track port)\n\n" +
	"To confirm the server came up, tail/cat the log path from `tracks " +
	"services` (the pane also tees its output there); do not assume " +
	"success just because `tracks up` returned.\n\n" +
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
	"reply and carry on with the actual work.\n\n" +
	"**Response style.** These sessions are read in a dashboard, not " +
	"a chat window — keep answers short.\n" +
	"  - Lead with the result or conclusion; drop preamble (\"I'll " +
	"now…\", \"Great question…\") and closing summaries that restate " +
	"what you just did.\n" +
	"  - Prefer bullet lists and tables over paragraphs. Use a table " +
	"when comparing options, listing files with per-file notes, or " +
	"reporting multiple results.\n" +
	"  - One line per point. Don't restate the task back, and don't " +
	"narrate routine steps — the tool calls already show them.\n" +
	"  - Explain *why* only when it's non-obvious or you're proposing " +
	"a decision. No filler acknowledgements: answer, act, or ask.\n\n" +
	"**Code comments.** Do not add comments to code unless a comment " +
	"is essential to understand a non-obvious fix or behavior.\n" +
	"  - No comments that restate what the code plainly does, and no " +
	"\"changed this\" / \"added that\" narration of your own edit.\n" +
	"  - Match the existing comment density of the file — if the " +
	"surrounding code has no comments, add none.\n" +
	"  - A comment earns its place only when the *why* is genuinely " +
	"surprising (a workaround, an ordering constraint, a subtle edge " +
	"case)."

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

// draftPRRepos returns the names of the track's repos configured to
// open pull requests as drafts by default.
func draftPRRepos(cfg config.Config, repos []state.TrackRepo) []string {
	var out []string
	for _, r := range repos {
		if cr, ok := cfg.RepoByName(r.Name); ok && cr.DraftPRs {
			out = append(out, r.Name)
		}
	}
	return out
}

// draftPRSuffix builds the prompt fragment instructing Claude to open
// PRs as drafts. When every repo on the track wants drafts (the common
// single-repo case) it stays generic; otherwise it names the repos so a
// mixed-repo track only drafts the ones that opted in.
func draftPRSuffix(draftRepos []string, totalRepos int) string {
	if len(draftRepos) == totalRepos {
		return "\n\nWhen you open a pull request, open it as a **draft** " +
			"(`gh pr create --draft`) unless the user asks otherwise."
	}
	return "\n\nWhen you open a pull request for any of these repos, open it " +
		"as a **draft** (`gh pr create --draft`) unless the user asks " +
		"otherwise: " + strings.Join(draftRepos, ", ") + "."
}

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
		if draft := draftPRRepos(cfg, t.Repos); len(draft) > 0 {
			prompt += draftPRSuffix(draft, len(t.Repos))
		}
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
	if o.Resume {
		// --resume <sessionID> continues the existing conversation;
		// no positional prompt and no separate --session-id.
		claudeArgv = append(claudeArgv, "--resume", shellQuote(o.SessionID))
	} else {
		if o.TaskPrompt != "" {
			// Claude takes the prompt as a positional arg: it opens
			// the TUI pre-filled with that prompt.
			claudeArgv = append(claudeArgv, shellQuote(o.TaskPrompt))
		}
		if o.SessionID != "" {
			claudeArgv = append(claudeArgv, "--session-id", shellQuote(o.SessionID))
		}
	}
	if o.PermissionMode != "" {
		claudeArgv = append(claudeArgv, "--permission-mode", shellQuote(o.PermissionMode))
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

// BuildResumeOptions assembles SpawnOptions for continuing a finished track's
// Claude conversation via --resume. Unlike BuildOptions, no task prompt is
// assembled — the session picks up from its existing transcript. The track
// must have a non-empty SessionID; returns an error otherwise.
func BuildResumeOptions(cfg config.Config, t state.Track, socketDir, sentinelPath string) (SpawnOptions, error) {
	if t.SessionID == "" {
		return SpawnOptions{}, errors.New("track has no session ID; cannot resume")
	}
	addDirs := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		addDirs = append(addDirs, r.Path)
	}
	cwd := ""
	if len(t.Repos) > 0 {
		cwd = t.Repos[0].Path
	} else if home, err := os.UserHomeDir(); err == nil {
		cwd = home
	}
	return SpawnOptions{
		CLIBinary:      cfg.Claude.Binary,
		PermissionMode: cfg.Claude.PermissionMode,
		AddDirs:        addDirs,
		CWD:            cwd,
		TrackID:        t.ID,
		SocketDir:      socketDir,
		SentinelPath:   sentinelPath,
		SessionID:      t.SessionID,
		Resume:         true,
	}, nil
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
