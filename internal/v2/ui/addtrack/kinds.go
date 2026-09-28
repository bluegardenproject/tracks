package addtrack

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Kind is what a track does, picked on the form's Type row.
type Kind int

const (
	Work Kind = iota
	Ask
	Plan
	Review
	Doc
)

// control is a part of the form that takes focus.
type control int

const (
	ctlType control = iota
	ctlRepos
	ctlRepo
	ctlTarget
	ctlDocument
	ctlName
	ctlTerminal
	ctlSections
	ctlCandor
	ctlPrompt
	ctlCreate
	ctlCancel
)

// kindInfo is a kind's Type row entry, the text under the row, the
// prompt it starts with and its fields in order. about and prompt are
// v1's, word for word.
type kindInfo struct {
	label, about, prompt string
	fields               []control
}

var kinds = []kindInfo{
	Work: {
		label:  "Work",
		about:  "Creates a branch + worktree you edit on. The usual track.",
		fields: []control{ctlRepos, ctlName, ctlTerminal, ctlPrompt},
	},
	Ask: {
		label:  "Ask",
		about:  "Points Claude at your primary checkout read-only. Promote later to start editing.",
		fields: []control{ctlRepos, ctlName, ctlPrompt},
	},
	Plan: {
		label: "Plan",
		about: "Read-only planning against your primary checkout. Promote later to implement.",
		prompt: `Produce a detailed implementation plan for the following. This is a
read-only planning task — investigate the codebase and design an
approach; do not modify anything. When the user is ready to build it,
the track can be promoted to a worktree.

`,
		fields: []control{ctlRepos, ctlName, ctlPrompt},
	},
	Review: {
		label: "Review",
		about: "Checks out a PR/branch detached so the reviewer agent can diff it.",
		prompt: `Run a code review of the checked-out PR / branch against its base.

The worktree is already on the target you picked (detached at the PR
head or branch tip), so "the current branch against its base" is the
diff you want to review.

Invoke the dedicated review subagent rather than reviewing yourself:

  Task({
    subagent_type: "tracks-v2-reviewer",
    prompt: "Review the current branch against its base and report findings."
  })

The subagent is auto-discovered from the user's global Claude config —
no setup needed inside the worktree. It's read-only by design and ends
its report with one of ` + "`REVIEW OUTCOME: pass`" + ` or ` + "`REVIEW OUTCOME: blocked`" + `.

Present the subagent's findings verbatim and wait for follow-up.

This is a **read-only audit**:
- Do NOT push, commit, or open a PR.
- Do NOT change any Jira ticket status or assignee (skip the
  Jira-sync workflow described in the global tracks suffix).`,
		fields: []control{ctlRepo, ctlTarget, ctlName, ctlCandor, ctlPrompt},
	},
	Doc: {
		label: "Doc review",
		about: "Reviews a file on disk (md/pdf/image/csv): judges the argument and how it reads, and optionally fact-checks its claims against your repos, GitHub, and Jira.",
		prompt: `Review the document configured for this track.

Context for the review (optional — fill in or delete):
- Audience: who reads this, and what they decide from it
- Ground truth: tickets (e.g. ABC-123) or repo areas the claims should
  be checked against
- Known problems: anything already suspected to be wrong or stale
- Focus: sections that matter most, if some matter more than others`,
		fields: []control{ctlDocument, ctlRepos, ctlName, ctlSections, ctlCandor, ctlPrompt},
	},
}

// controls are kind's controls in focus order.
func controls(k Kind) []control {
	out := append([]control{ctlType}, kinds[k].fields...)
	return append(out, ctlCreate, ctlCancel)
}

func candorLabel(level int) string { return fmt.Sprintf("%d — %s", level, track.CandorLabel(level)) }

// sections are a doc review's optional parts, v1's labels word for word.
var sections = []string{
	"Opinion — is the argument sound, does the content hold up, does it read well",
	"Claim check — verify the claims against your repos, GitHub, and Jira",
}
