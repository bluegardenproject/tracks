package hooks

import "regexp"

var (
	// claudeDialog is the selected option of a dialog in Claude's
	// window, such as "❯ 1. Yes": its permission prompts, questions and
	// plan approvals all have one.
	claudeDialog = regexp.MustCompile(`(?m)^\s*❯\s*\d+\.\s`)
	// cursorDialog is the first option of Cursor's command approval,
	// "→ Run (once) (y)", or the title of its questions' box.
	cursorDialog = regexp.MustCompile(`(?m)^\s*(→ Run \(once\)|│ Clarifying Questions\s)`)
)

// DialogOpen reports whether screen, an engine's agent pane, shows a
// dialog; known is false for an engine whose dialogs Tracks can't
// tell apart.
func DialogOpen(engine, screen string) (open, known bool) {
	switch engine {
	case "claude":
		return claudeDialog.MatchString(screen), true
	case "cursor":
		return cursorDialog.MatchString(screen), true
	}
	return false, false
}

// ScreenOpens reports whether only the screen tells that engine's
// dialogs open: Cursor has no hook for it, unlike Claude.
func ScreenOpens(engine string) bool { return engine == "cursor" }
