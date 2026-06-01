package newtrack

// Template is one preset task prompt the new-track form can apply.
// We intentionally keep this list short — only patterns that
// recur often enough to be worth a button. Custom is always the
// default; the form falls back to free-form input when chosen.
type Template string

const (
	TemplateCustom Template = "custom"
	TemplateReview Template = "review"
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
	TemplateReview: `Run a code review of the current branch against its base.

Invoke the dedicated review subagent rather than reviewing yourself:

  Task({
    subagent_type: "tracks-reviewer",
    prompt: "Review the current branch against its base and report findings."
  })

The subagent is installed at .claude/agents/tracks-reviewer.md in this
worktree. It's read-only by design and will end its report with one of
` + "`REVIEW OUTCOME: pass`" + ` or ` + "`REVIEW OUTCOME: blocked`" + `.

Present the subagent's findings verbatim and wait for follow-up.

This is a **read-only audit**:
- Do NOT push, commit, or open a PR.
- Do NOT change any Jira ticket status or assignee (skip the
  Jira-sync workflow described in the global tracks suffix).`,
}

// templateLabels gives the picker its human-readable option text.
var templateLabels = map[Template]string{
	TemplateCustom: "Custom (free-form task prompt)",
	TemplateReview: "Review the current branch / PR",
}
