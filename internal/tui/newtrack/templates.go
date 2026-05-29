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
	TemplateReview: `Review the current branch (or the PR if one already exists) against its base branch.

- Use any review skills installed in this repository.
- Surface findings grouped by severity (block / warn / hint).
- Do NOT push, commit, or open a PR — this is a read-only review.
- Present the findings and wait for follow-up.`,
}

// templateLabels gives the picker its human-readable option text.
var templateLabels = map[Template]string{
	TemplateCustom: "Custom (free-form task prompt)",
	TemplateReview: "Review the current branch / PR",
}
