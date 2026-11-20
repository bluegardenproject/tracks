package claude

import "testing"

func TestAsksTrust(t *testing.T) {
	// As Claude Code 2.1.329 draws it, wrapped in a 120-column pane.
	screen := ` Accessing workspace:
 /private/var/folders/x/T/trustcheck
 Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source
 project, or work from your team). If not, take a moment to review what's in this folder first.
 ❯ No, exit
   Yes, I trust
   this folder
 Enter to confirm · Esc to cancel`
	if !AsksTrust(screen) {
		t.Error("the trust question wasn't seen")
	}
	if !AsksTrust("Do you trust the files in this folder?\n 1. Yes, proceed\n 2. No, exit") {
		t.Error("the older trust question wasn't seen")
	}
	if AsksTrust("Reviewing internal/review/packet.go… I trust this function's callers.") ||
		AsksTrust(`The matcher looks for "Yes, I trust this folder" anywhere on the screen.`) {
		t.Error("a reviewer's own words were taken for the question")
	}
}
