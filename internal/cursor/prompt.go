package cursor

import (
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agent"

	"github.com/bluegardenproject/tracks/internal/state"
)

// taskSuffix is appended to every work/review prompt, the Cursor
// counterpart to claude.taskSuffix.
//
// It is a rewrite rather than a copy, because the Claude version's
// central instruction — invoke the `tracks-reviewer` subagent via the
// Task tool — names a mechanism Cursor does not have. Cursor has no
// user-definable subagents: ~/.cursor/agents exists but is empty with
// no documented format, and the `exploreSubagentModel` in its config is
// internal.
//
// So the review gate is preserved in intent and weakened in mechanism:
// the agent reviews its own diff in-conversation instead of handing it
// to a reviewer with a separate context. That is a real difference in
// rigour, not a wording change — a self-review shares the blind spots
// of the work it is reviewing. It is called out in
// docs/design/cursor-integration.md §10 and is the honest option until
// Cursor grows an equivalent.
//
// The wording overlaps claude.taskSuffix in the parts that describe
// tracks rather than the assistant (the interactive framing, the PR
// marker contract, the terseness ask). That duplication is known and
// deferred: splitting the Claude constant apart risks changing its text
// by accident, which is a prompt change disguised as a refactor.
const taskSuffix = "" +
	"You're running interactively inside a `tracks` worktree (the " +
	"TRACKS_ID env var is set). The user can switch into this tmux " +
	"pane at any time to reply. Stay engaged: if the task naturally " +
	"ends with a question or a confirmation, ask it and wait — do " +
	"NOT wrap up the session just to acknowledge completion.\n\n" +
	"**Review your own work before pushing (code changes only).** " +
	"Before you run `git push` or open a pull request, check what the " +
	"branch actually changes with `git diff --name-only <base>...HEAD`:\n" +
	"  - If every changed path is documentation or agent " +
	"configuration — `*.md`, `*.mdx`, `*.txt`, `docs/**`, `.cursor/**`, " +
	"`.claude/**`, `AGENTS.md`, `CLAUDE.md`, `LICENSE` — skip the " +
	"review and push. Say in one line that you skipped it because the " +
	"diff is docs-only.\n" +
	"  - Otherwise (any source, config, test, schema, lockfile, build " +
	"or CI change, including a diff that mixes those with docs) review " +
	"before pushing. When in doubt, review.\n\n" +
	"The review:\n" +
	"  1. Run `git diff <base>...HEAD` and read the whole diff, not " +
	"just the file list.\n" +
	"  2. Report what you find grouped as block / warn / hint, against " +
	"whatever conventions the repo documents. Correctness bugs first, " +
	"then anything the repo's own patterns would reject. Be specific " +
	"about what you checked — a review that finds nothing should say " +
	"what it looked for.\n" +
	"  3. End with the literal line `REVIEW OUTCOME: pass` or " +
	"`REVIEW OUTCOME: blocked`, so the outcome is greppable rather " +
	"than a matter of tone.\n" +
	"  4. If you found blocks, do NOT push. Fix them, re-run the " +
	"review, and only push once it comes back `pass`.\n\n" +
	"If you open a pull request at any point, include the URL on its " +
	"own line as `TRACKS_PR_URL=<url>` so the tracks dashboard " +
	"surfaces it. If you open several, emit one such line per PR.\n\n" + agent.DevServerContract + "\n\n" +
	"Keep your output terse and your diffs comment-free unless the " +
	"repo's conventions ask otherwise — these sessions are read in a " +
	"dashboard."

// docReviewSuffix frames a document review for Cursor.
//
// Shorter than Claude's template only where Claude delegates to the
// tracks-docs-reviewer subagent. Everything that bounds what the track
// may touch — the write contract, the save flow, the response style —
// is the shared text from internal/agent, not a paraphrase. The first
// version of this function paraphrased it and lost the write contract
// entirely, which on a doc track means the user's primary checkouts
// are attached with nothing telling the agent to leave them alone.
func docReviewSuffix(t state.Track) string {
	return fmt.Sprintf(""+
		"You're running interactively inside a `tracks` doc-review track. "+
		"The user can switch into this tmux pane at any time to reply.\n\n"+
		"**The document under review is:** `%[1]s`\n\n"+
		"Read it in full before commenting. Judge the argument, the "+
		"content, the order it is told in, and how well it reads. Report "+
		"findings grouped by severity alongside what the document does "+
		"well, and be concrete: quote the line you mean. End your report "+
		"with a `DOC REVIEW OUTCOME:` line.\n\n"+
		"**Review brief.** These are the user's settings for this "+
		"review. They are not defaults for you to reinterpret, and a "+
		"section switched off must stay off:\n\n"+
		"%[2]s\n\n"+
		"Candor governs wording only. It never changes which findings "+
		"the review reports, their severity, or the verdict.\n\n"+
		"%[3]s\n\n"+
		"%[4]s\n\n"+
		"%[5]s",
		t.DocPath(), docReviewBrief(t),
		agent.DocSaveFlow, agent.DocWriteContract, agent.DocResponseStyle)
}

// docReviewBrief renders the review settings the model must honour.
//
// Both optional sections are stated explicitly, ON as well as OFF. An
// omitted line reads as "unspecified" and invites the model to fall
// back to its own default — which is how a section the user switched
// off runs anyway.
func docReviewBrief(t state.Track) string {
	onOff := func(on bool) string {
		if on {
			return "ON"
		}
		return "OFF"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  - Candor level: %d/10\n", t.CandorLevel())
	fmt.Fprintf(&b, "  - Opinion section: %s\n", onOff(!t.SkipOpinion()))
	fmt.Fprintf(&b, "  - Claim check section: %s", onOff(!t.SkipClaimCheck()))
	return b.String()
}

// reviewCandorSuffix carries the candor level onto a code-review track.
//
// Claude's equivalent tells the model to pass the level to the review
// subagent; here the model is the reviewer, so it is addressed
// directly. Without this the level the creation form collects is
// silently discarded.
func reviewCandorSuffix(level int) string {
	return fmt.Sprintf("\n\n**Review candor: %d/10** (%s) — 1 is radical "+
		"candor, 10 is honest but gently framed. Candor governs wording "+
		"only: it never changes which findings you report, their "+
		"severity, or the verdict.",
		level, state.CandorLabel(level))
}

// homeDir is the pane's directory of last resort, so tmux always has a
// valid one to open in.
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}
