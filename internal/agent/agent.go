// Package agent holds what every agent CLI tracks can launch has in
// common: the shell scaffolding a track's tmux pane runs, and the
// prompt fragments that describe tracks itself rather than any
// particular assistant.
//
// The split is deliberate and narrow. A provider package
// (internal/claude, internal/cursor) owns its own flags, its own
// permission vocabulary, and any instruction naming a mechanism only
// that harness has. Everything here is either shell mechanics that
// must be byte-identical between providers, or a statement about how
// tracks works that stays true whoever is reading it.
//
// Notably NOT here: the review-candor fragment, which tells the model
// to pass a line to a review *subagent*, and Claude's permission-mode
// clamping. Both read as generic and are not.
package agent

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/state"
)

// Wrapper is the shell scaffolding tracks puts around an agent command
// inside a tmux pane: the environment every track exports, the
// sentinel that tells the supervisor the agent exited, and the login
// shell the pane falls back to so it stays usable afterwards.
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

	// SentinelPath is touched the instant the agent exits, so the
	// supervisor can finalize the track without depending on pid death.
	// Empty means no sentinel handling.
	SentinelPath string
}

// Command wraps an already-assembled agent command line — flags
// quoted, joined, provider's business — in that scaffolding, and
// returns the single string tmux should run in the new pane.
//
// tmux's `new-window <command>` hands its argument to /bin/sh -c, so
// this must produce a string rather than an argv.
func (w Wrapper) Command(line CommandLine) string {
	inner := string(line)
	if w.SentinelPath != "" {
		inner += "\ntouch " + shellx.Quote(w.SentinelPath)
	}
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

// ReadOnlySuffix is appended for worktree-less (ask/plan) tracks. They
// point at the user's PRIMARY checkout — the one their editor watches
// — so the prompt makes the read-only contract explicit as a second
// line of defence behind whatever read-only mode the provider offers.
// For Claude that mode is a default rather than a hard sandbox;
// whether Cursor's --mode plan behaves the same has not been
// established, so this fragment carries the contract on its own.
const ReadOnlySuffix = "" +
	"\n\n**This is a read-only track.** You are pointed at the user's " +
	"primary checkout — the working copy their editor uses — NOT a " +
	"throwaway worktree. Do not modify any files, create branches, or " +
	"run mutating commands; investigate and answer (or produce a plan) " +
	"only. When the user is ready to implement, the track can be " +
	"promoted to its own worktree with `tracks promote <id>`."

// DraftPRRepos returns the names of the track's repos configured to
// open pull requests as drafts by default.
func DraftPRRepos(cfg config.Config, repos []state.TrackRepo) []string {
	var out []string
	for _, r := range repos {
		if cr, ok := cfg.RepoByName(r.Name); ok && cr.DraftPRs {
			out = append(out, r.Name)
		}
	}
	return out
}

// DraftPRSuffix builds the prompt fragment instructing the agent to
// open PRs as drafts. When every repo on the track wants drafts (the
// common single-repo case) it stays generic; otherwise it names the
// repos so a mixed-repo track only drafts the ones that opted in.
func DraftPRSuffix(draftRepos []string, totalRepos int) string {
	if len(draftRepos) == totalRepos {
		return "\n\nWhen you open a pull request, open it as a **draft** " +
			"(`gh pr create --draft`) unless the user asks otherwise."
	}
	return "\n\nWhen you open a pull request for any of these repos, open it " +
		"as a **draft** (`gh pr create --draft`) unless the user asks " +
		"otherwise: " + strings.Join(draftRepos, ", ") + "."
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

// The doc-review paragraphs below are shared by every provider. They
// describe what a doc-review track may touch and how it reports —
// statements about tracks, not about any assistant.
//
// They are constants rather than prose duplicated per provider because
// the failure mode is silent: the first Cursor implementation of this
// path simply omitted the write contract, and nothing failed. A
// provider that composes these cannot lose a clause without a test
// noticing.

// DocWriteContract bounds what a doc-review track may modify.
//
// This is the main thing standing between the agent and the user's
// primary checkouts. A doc track attaches them with --add-dir for
// grounding, and whatever per-write prompting the provider offers is a
// backstop to this text, not a replacement for it.
const DocWriteContract = "" +
	"**Write contract.** That report file is the ONLY file you may " +
	"create or modify in this track. Any repos attached to this track " +
	"are the user's PRIMARY checkouts — the working copies their editor " +
	"watches — and are attached solely as read-only ground truth for " +
	"checking the document's claims. Never edit them, never commit, " +
	"never push, never open a PR, and never change a Jira ticket's " +
	"status or assignee (this is a read-only audit)."

// DocSaveFlow is the confirm-before-writing sequence, including the
// refusal to overwrite a previous review.
const DocSaveFlow = "" +
	"**Then ask whether to save it.** After presenting the report, ask " +
	"the user a single question: whether to write it to a markdown file " +
	"next to the document (`<document-basename>.review.md`). Wait for " +
	"the answer.\n" +
	"  - Only write the file if they say yes. If they name a different " +
	"path, use that instead.\n" +
	"  - If the target file already exists, read it first and offer a " +
	"dated name (`<basename>.review-YYYY-MM-DD.md`) rather than " +
	"overwriting a previous review.\n" +
	"  - The saved file is the report as presented, plus a header line " +
	"naming the reviewed document and the date."

// DocResponseStyle keeps a doc review readable in the dashboard.
const DocResponseStyle = "" +
	"**Response style.** These sessions are read in a dashboard — no " +
	"preamble, no closing summary restating the report. Lead with the " +
	"report itself."

// DevServerContract tells the agent to start dev servers through
// tracks rather than in its own pane.
//
// Shared because the capability is tracks', not the assistant's: the
// pane env reaches every provider identically, so an agent without
// this text has the capability and no idea it exists. The failure it
// prevents is specific — asked to start the dev server, the agent runs
// `pnpm dev` in its own pane and blocks, or backgrounds it with `&`
// and loses the output.
const DevServerContract = "" +
	"**Dev-server services.** When the user asks you to start (or run, " +
	"boot, spin up) the dev server, do NOT run `pnpm dev` / `npm " +
	"start` / `pnpm install` yourself, and never background a server process (`… &` / `nohup`). Run `tracks up <name>` instead: " +
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
	"  - `tracks up` (no arg) starts ALL the track's services, each in its own pane — use this when asked to run the dev servers; `tracks up <name>` starts just one (its " +
	"depends_on services first)\n" +
	"  - `tracks down <name>` stops a running service\n" +
	"  - `tracks url <name>` prints the URL (stable proxy + track port)\n\n" +
	"To confirm the server came up, tail/cat the log path from `tracks " +
	"services` (the pane also tees its output there); do not assume " +
	"success just because `tracks up` returned. If `tracks up` itself errors (command not found, daemon unreachable, unknown service, any non-zero exit), STOP and report the exact error to the user — do not fall back to starting the server yourself."
