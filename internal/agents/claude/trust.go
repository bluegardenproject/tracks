package claude

import "strings"

// trustQuestions are what Claude's question whether to trust a folder
// says, in the versions Tracks knows.
var trustQuestions = []string{
	"Yes, I trust this folder",
	"Is this a project you created or one you trust",
	"Do you trust the files in this folder",
}

// trustDecline is the question's option to leave, which every version
// Tracks knows shows: with it, the question's words are the question,
// not a reply quoting them.
const trustDecline = "No, exit"

// AsksTrust reports whether screen, a Claude pane's, shows Claude asking
// whether to trust its folder: a reviewer can't go on until someone
// answers it.
func AsksTrust(screen string) bool {
	screen = strings.Join(strings.Fields(screen), " ")
	if !strings.Contains(screen, trustDecline) {
		return false
	}
	for _, q := range trustQuestions {
		if strings.Contains(screen, q) {
			return true
		}
	}
	return false
}
