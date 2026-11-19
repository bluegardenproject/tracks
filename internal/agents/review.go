package agents

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/track"
)

// ReviewSteps is the pre-push review, the same on every engine:
// `tracks review` starts the track's reviewer, a session that hasn't
// seen the agent's conversation, in a hidden pane of the track. The
// agent waits for its report rather than reviewing its own work, and
// leaves a new review to a fresh reviewer.
const ReviewSteps = "" +
	"The review:\n" +
	"  1. Run `tracks review`. It starts this track's reviewer, an agent " +
	"session that has not seen this conversation, in a hidden pane of the " +
	"track, and prints its report when it's done. That can take minutes: " +
	"give the command the longest timeout your shell tool allows (in " +
	"Claude Code, the Bash tool's `timeout: 600000`). If the command is " +
	"stopped before the report comes, run `tracks review --wait`: the " +
	"reviewer runs on, and that picks its report up. Do NOT review the " +
	"diff yourself instead, and do NOT start a reviewer any other way.\n" +
	"  2. Read its report. It ends with `REVIEW OUTCOME: pass` or " +
	"`REVIEW OUTCOME: blocked`.\n" +
	"  3. If blocked, address every `block` finding and run `tracks " +
	"review` again: it ends the old reviewer and starts a fresh one. Do " +
	"not push with unresolved blocks.\n" +
	"  4. Acknowledge `warn` findings in the PR description and include " +
	"the final `REVIEW OUTCOME` line so the human reviewer knows what was " +
	"already vetted.\n" +
	"  5. If `tracks review` itself fails (command not found, no changes, " +
	"reviewer unavailable), say so plainly and review the diff yourself as " +
	"a fallback, ending with the same `REVIEW OUTCOME:` line — a degraded " +
	"review beats a skipped one, but say which you did.\n\n" +
	"You may summarise the report or explain a finding yourself, and the " +
	"user can ask the reviewer about its review in its pane, which the " +
	"footer's button shows. A new review, or one of another scope, is " +
	"always a new `tracks review`, never your own."

// ReviewTemplate is a Review track's task. The track's prompt also
// holds ReviewSteps, which say how the review runs. The user's prompt,
// which is optional, follows it.
const ReviewTemplate = "" +
	"Run a code review of the checked-out PR / branch against its base.\n\n" +
	"The worktree is already on the target you picked (detached at the " +
	"PR head or branch tip), so \"the current branch against its base\" " +
	"is the diff you want to review.\n\n" +
	"Run `tracks review`, as **The review** below describes, rather than " +
	"reviewing it yourself. Present its findings verbatim and wait for " +
	"follow-up.\n\n" +
	"This is a **read-only audit**:\n" +
	"- Do NOT edit files, commit, push, or open a PR.\n" +
	"- Do NOT change any Jira ticket status or assignee."

// ReviewCandor tells a Review track's agent the track's candor level.
// `tracks review` hands it to the reviewer; the agent only needs to
// know it doesn't change what the report says.
func ReviewCandor(level int) string {
	return fmt.Sprintf("\n\n**Review candor: %d/10** (%s) — 1 is radical "+
		"candor, 10 is honest but gently framed. `tracks review` hands "+
		"this level to the reviewer. Candor governs wording only: it never "+
		"changes which findings are reported, their severity, or the "+
		"pass/blocked verdict.",
		level, track.CandorLabel(level))
}
