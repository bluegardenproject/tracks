package agents

import "strings"

// The prompt texts below are v1's (internal/agent), word for word; a
// test compares them.

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

// TerminalContract tells agents how to open a user-facing shell without
// launching a nested or background terminal process in their own pane.
const TerminalContract = "" +
	"**Track terminal.** When the user asks for a terminal or shell in the " +
	"track's worktree, run `tracks terminal`. It opens an interactive shell " +
	"in the track window's right-hand pane column and returns immediately. " +
	"`$TRACKS_ID` is already set, so do not pass `--track`."

// LinksContract is v2's own, not v1's: every start prompt carries it,
// right after the task. It has no apostrophes, so quoting leaves it as
// written and the v1 parity test can find it.
//
// It exists because an agent that can't read a linked ticket or page
// tends to plan from the rest of the prompt instead, and the link is
// often where the requirements are.
const LinksContract = "" +
	"**Links you cannot read.** If the task links to something you " +
	"cannot open (a Jira ticket, a Confluence page, a Google Doc, a " +
	"Figma file, a GitHub page) because an MCP server is missing or not " +
	"authenticated, or the page needs a login, STOP before you plan, " +
	"explore or change anything. Do not guess what it says and do not " +
	"make a plan from the rest of the prompt: the link usually holds " +
	"the requirements. Tell the user which link failed and why, then " +
	"ask one question: should you wait while they authenticate or add " +
	"the MCP server, or skip the link and go on without it? Wait for " +
	"the answer. If they authenticated, read the link again before " +
	"anything else. The same applies when a subagent reports that it " +
	"could not read a link."

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
