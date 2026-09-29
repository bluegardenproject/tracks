package hooks

import "regexp"

// claudeDialog is the selected option of a dialog in Claude's window,
// such as "❯ 1. Yes": its permission prompts, questions and plan
// approvals all have one.
var claudeDialog = regexp.MustCompile(`(?m)^\s*❯\s*\d+\.\s`)

// DialogOpen reports whether screen, an engine's agent pane, shows a
// dialog; known is false for an engine whose dialogs Tracks can't
// tell apart yet.
func DialogOpen(engine, screen string) (open, known bool) {
	switch engine {
	case "claude":
		return claudeDialog.MatchString(screen), true
	}
	return false, false
}
