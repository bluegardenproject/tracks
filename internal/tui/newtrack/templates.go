package newtrack

// Template is one preset task prompt the new-track form can apply.
// We intentionally keep this list short — only patterns that
// recur often enough to be worth a button. Custom is always the
// default; the form falls back to free-form input when chosen.
type Template string

const (
	TemplateCustom    Template = "custom"
	TemplateReview    Template = "review"
	TemplateDocReview Template = "docreview"
	TemplateAsk       Template = "ask"
	TemplatePlan      Template = "plan"
	TemplateResume    Template = "resume"
)

// templatePrompts maps a Template to the body that pre-fills the
// task-prompt field when the user picks it. Edit these in place to
// tweak the wording — the form re-renders on every keystroke so
// changes are immediate after rebuild.
//
// Templates are deliberately repo-agnostic: tracks ships generically
// and per-project review behavior comes from whatever review skills
// each repo has installed.
var templatePrompts = map[Template]string{
	TemplateCustom: "",
	// Ask is sent verbatim — no preamble. The read-only contract is
	// enforced by plan permission mode (and a short suffix when repos
	// are attached), so wrapping the question in framing here only
	// risks steering the answer. See claude.BuildOptions.
	TemplateAsk: "",
	TemplatePlan: `Produce a detailed implementation plan for the following. This is a
read-only planning task — investigate the codebase and design an
approach; do not modify anything. When the user is ready to build it,
the track can be promoted to a worktree.

`,
	TemplateReview: `Run a code review of the checked-out PR / branch against its base.

The worktree is already on the target you picked (detached at the PR
head or branch tip), so "the current branch against its base" is the
diff you want to review.

Invoke the dedicated review subagent rather than reviewing yourself:

  Task({
    subagent_type: "tracks-reviewer",
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
	// Doc review is deliberately thin: the subagent invocation, the
	// write contract, and the save-the-report protocol are all appended
	// by the daemon for KindDoc tracks (see claude.docReviewTemplate),
	// so they survive the user editing this prompt. What's left is the
	// part only the user knows — who reads the document and what should
	// be treated as ground truth.
	TemplateDocReview: `Review the document configured for this track.

Context for the review (optional — fill in or delete):
- Audience: who reads this, and what they decide from it
- Ground truth: tickets (e.g. ABC-123) or repo areas the claims should
  be checked against
- Known problems: anything already suspected to be wrong or stale
- Focus: sections that matter most, if some matter more than others`,
}

// templateLabels gives the picker its human-readable option text.
var templateLabels = map[Template]string{
	TemplateCustom:    "Work — branch + worktree to implement a change",
	TemplateAsk:       "Ask — read-only question about the code (no worktree)",
	TemplatePlan:      "Plan — read-only implementation plan (no worktree)",
	TemplateReview:    "Review — a PR or branch",
	TemplateDocReview: "Doc review — a local doc, spec, or deck",
	TemplateResume:    "Resume — continue a finished track's Claude session",
}

// templateDescriptions give the picker a one-line hint under each
// option so the read-only / worktree-less behaviour is discoverable.
var templateDescriptions = map[Template]string{
	TemplateCustom:    "Creates a branch + worktree you edit on. The usual track.",
	TemplateAsk:       "Points Claude at your primary checkout read-only. Promote later to start editing.",
	TemplatePlan:      "Read-only planning against your primary checkout. Promote later to implement.",
	TemplateReview:    "Checks out a PR/branch detached so the reviewer agent can diff it.",
	TemplateDocReview: "Reviews a file on disk (md/pdf/image/csv): judges the argument and how it reads, and optionally fact-checks its claims against your repos, GitHub, and Jira.",
	TemplateResume:    "Picks a finished track — including ones interrupted by a quit — and reopens its Claude conversation.",
}

// kindFor maps a Template to the daemon track Kind string.
func kindFor(t Template) string {
	switch t {
	case TemplateAsk:
		return "ask"
	case TemplatePlan:
		return "plan"
	case TemplateReview:
		return "review"
	case TemplateDocReview:
		return "doc"
	default:
		return "work"
	}
}
