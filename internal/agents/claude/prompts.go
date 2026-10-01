package claude

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
)

// taskSuffix is appended to every task prompt the daemon sends to
// Claude. Hardcoded here so we can update the wording with the
// binary instead of asking every user to edit YAML.
//
// Key concerns:
//
//  1. Make sure Claude knows it's running interactively (don't
//     treat every task as one-shot).
//  2. Mandate a code-review gate before any push / PR when the
//     diff contains code — the review uses whatever conventions
//     the repo documents. Docs- and agent-config-only diffs skip
//     it: there is nothing for a code reviewer to find, and the
//     round-trip is pure latency.
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
	"**Mandatory pre-push review (code changes only).** Before you " +
	"run `git push` or open a pull request, check what the branch " +
	"actually changes with `git diff --name-only <base>...HEAD`:\n" +
	"  - If every changed path is documentation or agent " +
	"configuration — `*.md`, `*.mdx`, `*.txt`, `docs/**`, " +
	"`.claude/**`, `AGENTS.md`, `CLAUDE.md`, `LICENSE` — skip the " +
	"review and push. Say in one line that you skipped it because " +
	"the diff is docs-only.\n" +
	"  - Otherwise (any source, config, test, schema, lockfile, " +
	"build or CI change, including a diff that mixes those with " +
	"docs) the review below is mandatory. When in doubt, review.\n\n" +
	"The review:\n" +
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
	"as `TRACKS_PR_URL=<url>` so the tracks dashboard surfaces it. " +
	"If you open several, emit one such line per PR — the dashboard " +
	"tracks each one and rolls them up into the track's status.\n\n" +
	agents.TerminalContract + "\n\n" +
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
	"  4. An error while assigning or moving the ticket is " +
	"non-fatal — note it in your reply and carry on with the actual " +
	"work. A ticket you cannot read is not: follow **Links you cannot " +
	"read**.\n\n" +
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

// docReviewTemplate is appended for KindDoc tracks. It owns the two
// invariants of a doc review that must survive the user editing the
// task prompt: the write contract (exactly one file may be written,
// and only on request) and the save protocol at the end.
//
// Unlike ask/plan, a doc-review track does NOT run in plan permission
// mode — saving the report is a real write, and an ExitPlanMode
// "approve to implement" dialog is the wrong framing for it. The save
// confirmation is therefore the pane question below, not a permission
// prompt: no permission mode guarantees one (auto may auto-approve,
// acceptEdits and bypassPermissions never prompt), so the protocol has
// to carry it. docPermissionMode clamps the permissive modes to a
// prompting one as a backstop, which leaves this contract the main thing
// standing between Claude and the attached primary checkouts.
//
// %[1]s is the resolved document path; %[2]s is the review brief from
// docReviewBrief (candor level and which optional sections to run).
const docReviewTemplate = "" +
	"You're running interactively inside a `tracks` doc-review track. " +
	"The user can switch into this tmux pane at any time to reply.\n\n" +
	"**The document under review is:** `%[1]s`\n\n" +
	"Run the review through the dedicated subagent rather than " +
	"reviewing it yourself:\n\n" +
	"    Task({ subagent_type: \"tracks-docs-reviewer\", prompt: " +
	"\"Review the document at <path>. Repos attached for grounding: " +
	"<names or none>.\" })\n\n" +
	"**Review brief.** These are the user's settings for this review. " +
	"Append them verbatim to the subagent's prompt — they are not " +
	"defaults for you to reinterpret, and a section switched off must " +
	"stay off:\n\n" +
	"%[2]s\n\n" +
	"Candor governs wording only. It never changes which findings the " +
	"review reports, their severity, or the verdict.\n\n" +
	"The subagent is auto-discovered from the user's global Claude " +
	"config — no setup needed. It is read-only and ends its report " +
	"with a `DOC REVIEW OUTCOME:` line.\n\n" +
	"Present its report **verbatim** in the pane — do not re-summarize, " +
	"re-rank, or soften it.\n\n" +
	agents.DocSaveFlow + "\n\n" +
	agents.DocWriteContract + "\n\n" +
	agents.DocResponseStyle

// docReviewBrief renders the indented block of review settings the
// caller must forward to the docs-reviewer subagent: the candor level
// and whether each optional section runs.
//
// Both switches are stated explicitly, ON as well as OFF. An omitted
// line reads as "unspecified" and invites the subagent to fall back to
// its own default, which is exactly the reinterpretation the brief
// exists to prevent.
func docReviewBrief(t track.Track) string {
	level := track.CandorLevel(t.Candor)
	lines := []string{
		fmt.Sprintf("    Candor level: %d/10 (%s). 1 = radical candor, 10 = honest but gently framed.", level, track.CandorLabel(level)),
	}
	if !t.Opinion {
		lines = append(lines, "    Opinion section: OFF — skip it; report findings only.")
	} else {
		lines = append(lines, "    Opinion section: ON — judge the argument, the reasoning, "+
			"whether the content holds up, and how easily it reads for its audience.")
	}
	if !t.ClaimCheck {
		lines = append(lines, "    Claim check: OFF — do not verify claims. No repo, GitHub, or Jira "+
			"lookups and no claim-check table; flag a claim that looks shaky as an unverified `warn` instead.")
	} else {
		lines = append(lines, "    Claim check: ON — verify the load-bearing claims and report the claim-check table.")
	}
	return strings.Join(lines, "\n")
}

// reviewCandorSuffix is appended for KindReview (code) tracks so the
// candor level reaches tracks-reviewer the same way it reaches the docs
// reviewer. Kept short: unlike a doc review, nothing else about a code
// review's shape is configurable.
func reviewCandorSuffix(level int) string {
	return fmt.Sprintf("\n\n**Review candor: %d/10** (%s) — 1 is radical "+
		"candor, 10 is honest but gently framed. Include the line "+
		"`Candor level: %d/10` in the review subagent's prompt. Candor "+
		"governs wording only: it never changes which findings are "+
		"reported, their severity, or the pass/blocked verdict.",
		level, track.CandorLabel(level), level)
}

// docPermissionMode clamps the configured permission mode for a
// doc-review track.
//
// A doc track is deliberately not put in plan mode — saving the report
// is a real write, and an ExitPlanMode "approve to implement" dialog is
// the wrong framing for it (the save confirmation is the pane question
// docReviewTemplate mandates). But its write contract is prose, and the
// track carries --add-dir grants on the user's PRIMARY checkouts, so
// the modes that never prompt are clamped to one that does: acceptEdits
// and bypassPermissions skip prompting entirely, and auto's classifier
// may auto-approve a write. A configured "plan" is stricter than we
// need, so it's left alone.
//
// Shared with BuildResumeOptions: resuming a doc track restores those
// same grants, so it must not restore a non-prompting mode with them.
func docPermissionMode(configured string) string {
	if configured == "plan" {
		return "plan"
	}
	return "default"
}
