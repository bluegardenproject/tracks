package hooks

import (
	"regexp"
	"strings"
)

var (
	// claudeDialog is the selected option of a dialog in Claude's
	// window, such as "❯ 1. Yes": its permission prompts, questions and
	// plan approvals all have one.
	claudeDialog = regexp.MustCompile(`(?m)^\s*❯\s*\d+\.\s`)
	// cursorDialog is the first option of Cursor's command approval,
	// "→ Run (once) (y)", the title of its questions' box or of a plan
	// awaiting approval, or the option to approve a mode switch. An
	// accepted plan is titled "Plan", and an answered switch leaves
	// "SwitchMode → Agent" in the chat.
	cursorDialog = regexp.MustCompile(`(?m)^\s*(→ Run \(once\)|│ Clarifying Questions\s|│ Suggested Plan\s|(│\s*)?(→\s+)?Approve mode switch \(y\))`)

	readyToBuild = regexp.MustCompile(`^\s*│?\s*Ready to build\?\s*│?\s*$`)
	planOption   = regexp.MustCompile(`^(→\s+)?\d\.\s+(Yes, build locally|Yes, build in cloud|No, propose changes)\s`)
	boxBottom    = regexp.MustCompile(`^└─*┘$`)
)

// DialogOpen reports whether screen, an engine's agent pane, shows a
// dialog; known is false for an engine whose dialogs Tracks can't
// tell apart.
func DialogOpen(engine, screen string) (open, known bool) {
	switch engine {
	case "claude":
		return claudeDialog.MatchString(screen), true
	case "cursor":
		return cursorDialog.MatchString(screen) || planAwaits(screen), true
	}
	return false, false
}

// planAwaits reports whether a Cursor plan too long for its title to
// show still awaits approval. Its options, "→ 1. Yes, build locally
// (b)" and the others, stay on screen once it's accepted, but only an
// awaiting plan has nothing below its box: Cursor hides the prompt
// until the decision.
func planAwaits(screen string) bool {
	lines := strings.Split(screen, "\n")
	last := -1
	for i, line := range lines {
		if readyToBuild.MatchString(line) {
			last = i
		}
	}
	if last < 0 {
		return false
	}
	options, closed := 0, false
	for _, line := range lines[last+1:] {
		line = strings.TrimSpace(line)
		inner := strings.TrimSpace(strings.Trim(line, "│"))
		switch {
		case closed:
			if line != "" {
				return false
			}
		case boxBottom.MatchString(line):
			closed = true
		case inner == "":
		case planOption.MatchString(inner + " "):
			options++
		default:
			return false
		}
	}
	return options > 0
}

// ScreenOpens reports whether only the screen tells that engine's
// dialogs open: Cursor has no hook for it, unlike Claude.
func ScreenOpens(engine string) bool { return engine == "cursor" }
