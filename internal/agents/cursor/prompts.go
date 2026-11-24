package cursor

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
)

// taskSuffix is appended to every work/review prompt, the Cursor
// counterpart to claude.taskSuffix. The review runs the same way on
// both, through agents.ReviewSteps.
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
	"**Review before pushing (code changes only).** " +
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
	agents.ReviewSteps + "\n\n" +
	"**Ask before you open a pull request.** Commit and push as the work " +
	"needs, but open a pull request only after the user agreed to it in " +
	"this session, or the task asked for one; otherwise ask them when " +
	"the work is ready.\n\n" +
	"If you open a pull request at any point, include the URL on its " +
	"own line as `TRACKS_PR_URL=<url>` so the tracks dashboard " +
	"surfaces it. If you open several, emit one such line per PR.\n\n" +
	agents.TerminalContract + "\n\n" +
	agents.RelayContract + "\n\n" +
	"Keep your output terse and your diffs comment-free unless the " +
	"repo's conventions ask otherwise — these sessions are read in a " +
	"dashboard."

// docReviewSuffix frames a document review for Cursor.
//
// Shorter than Claude's template only where Claude delegates to the
// tracks-docs-reviewer subagents. Everything that bounds what the track
// may touch — the write contract, the save flow, the response style —
// is the shared text from internal/agent, not a paraphrase. The first
// version of this function paraphrased it and lost the write contract
// entirely, which on a doc track means the user's primary checkouts
// are attached with nothing telling the agent to leave them alone.
func docReviewSuffix(t track.Track) string {
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
		t.Document, docReviewBrief(t),
		agents.DocSaveFlow, agents.DocWriteContract, agents.DocResponseStyle)
}

// docReviewBrief renders the review settings the model must honour.
//
// Both optional sections are stated explicitly, ON as well as OFF. An
// omitted line reads as "unspecified" and invites the model to fall
// back to its own default — which is how a section the user switched
// off runs anyway.
func docReviewBrief(t track.Track) string {
	onOff := func(on bool) string {
		if on {
			return "ON"
		}
		return "OFF"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  - Candor level: %d/10\n", track.CandorLevel(t.Candor))
	fmt.Fprintf(&b, "  - Opinion section: %s\n", onOff(t.Opinion))
	fmt.Fprintf(&b, "  - Claim check section: %s", onOff(t.ClaimCheck))
	return b.String()
}
