package newtrack

// Template is one preset task prompt the new-track form can apply.
// We intentionally keep this list short — only patterns that
// recur often enough to be worth a button. Custom is always the
// default; the form falls back to free-form input when chosen.
type Template string

const (
	TemplateCustom Template = "custom"
	TemplateReview Template = "review"
	TemplateAsk    Template = "ask"
	TemplatePlan   Template = "plan"
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
	TemplateAsk: `Answer the following question about the codebase. This is a
read-only investigation — explore, read, and explain; do not modify
anything.

`,
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
}

// templateLabels gives the picker its human-readable option text.
var templateLabels = map[Template]string{
	TemplateCustom: "Work — branch + worktree to implement a change",
	TemplateAsk:    "Ask — read-only question about the code (no worktree)",
	TemplatePlan:   "Plan — read-only implementation plan (no worktree)",
	TemplateReview: "Review — a PR or branch",
}

// templateDescriptions give the picker a one-line hint under each
// option so the read-only / worktree-less behaviour is discoverable.
var templateDescriptions = map[Template]string{
	TemplateCustom: "Creates a branch + worktree you edit on. The usual track.",
	TemplateAsk:    "Points Claude at your primary checkout read-only. Promote later to start editing.",
	TemplatePlan:   "Read-only planning against your primary checkout. Promote later to implement.",
	TemplateReview: "Checks out a PR/branch detached so the reviewer agent can diff it.",
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
	default:
		return "work"
	}
}
