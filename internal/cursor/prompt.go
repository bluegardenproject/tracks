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
// Task tool — names a mechanism the Cursor CLI does not expose. Its
// Task tool accepts only built-in types, and invoking a custom agent
// by slash loads the definition into the *same* conversation.
//
// The gate is preserved by other means: `tracks review` runs the same
// reviewer definition as a second agent process with a fresh chat, so
// the isolation comes from the process boundary. The agent is told to
// run that command rather than to review its own work, and rather than
// to spawn the reviewer itself — keeping the spawn inside tracks is
// what lets it refuse a review from inside a review.
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
	"The review:\n" +
	"  1. Run `tracks review`. It runs the reviewer as a separate " +
	"agent session that has not seen this conversation, so it reads " +
	"your diff with fresh eyes rather than re-reading your own " +
	"reasoning. Do NOT review the diff yourself instead, and do NOT " +
	"invoke `agent` directly — `tracks review` is what stops a " +
	"reviewer reviewing itself.\n" +
	"  2. Read its report. It ends with `REVIEW OUTCOME: pass` or " +
	"`REVIEW OUTCOME: blocked`.\n" +
	"  3. If blocked, address every `block` finding and run " +
	"`tracks review` again. Do not push with unresolved blocks.\n" +
	"  4. If `tracks review` itself fails (command not found, no " +
	"changes, reviewer unavailable), say so plainly and review the " +
	"diff yourself as a fallback, ending with the same " +
	"`REVIEW OUTCOME:` line — a degraded review beats a skipped " +
	"one, but say which you did.\n\n" +
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
