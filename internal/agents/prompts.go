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

// TerminalContract tells agents how to open a user-facing shell without
// launching a nested or background terminal process in their own pane.
const TerminalContract = "" +
	"**Track terminal.** When the user asks for a terminal or shell in the " +
	"track's worktree, run `tracks terminal`. It opens an interactive shell " +
	"in the track window's right-hand pane column and returns immediately. " +
	"`$TRACKS_ID` is already set, so do not pass `--track`."

// SetupContract is in the prompt of a Work or Review track whose repos
// have a setup. Tracks runs the setup in a pane while the agent works,
// so the agent waits for it instead of installing on its own, and
// leaves a failure to the user.
const SetupContract = "" +
	"**Setup.** Tracks runs this track's setup (such as installing " +
	"dependencies) in a pane of the track's window, which may still be " +
	"running. Before you build, test, run or install anything, run " +
	"`tracks setup --wait`: it waits until the setup has finished, and " +
	"starts it first if it hasn't run yet. Never install dependencies " +
	"yourself while it runs. If it reports a failure, tell the user what " +
	"failed and wait: don't fix it, run it again or work around it unless " +
	"they ask. Run `tracks setup` to run a failed setup again only when " +
	"the user asks."

// DevServersContract is in the prompt of a Work or Review track whose
// repos have dev servers. Tracks runs them, on ports it picks, behind
// the user's proxy, so the agent goes through tracks, and only when the
// user asks, instead of starting one itself; and a server's error may be
// expected, so the agent leaves what happens next to the user.
const DevServersContract = "" +
	"**Dev servers.** Tracks runs this track's dev servers, each in a " +
	"pane of the track's window, on a port it picks. Start them only " +
	"when the user asks: `tracks up`, or one with `tracks up <server>`, " +
	"and stop them with `tracks down [server]`. `tracks servers` lists " +
	"them with their state and port, `tracks url <server>` prints one's " +
	"address, and `tracks logs <server>` shows what one printed. Never " +
	"start a dev server any other way, such as with `pnpm dev`. " +
	"If a server doesn't start, crashes or reports an error, tell the " +
	"user what `tracks up` printed or `tracks logs` shows and wait: don't " +
	"restart it, change it or work around it unless they ask. The error " +
	"may be expected while other work is pending."

// LinksContract is in every start prompt, right after the task.
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
