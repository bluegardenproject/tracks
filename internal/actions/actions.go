// Package actions are the track actions: buttons on the footer's top
// row, each for the track whose window is shown, that send its agent a
// prompt. It says which actions there are, what they send and when a
// sent one may be sent again; the daemon and the footer do the rest.
package actions

import (
	"embed"
	"slices"
	"strings"
)

// PRComments has the agent work through the open review comments on
// the track's pull requests. The ID names the footer's click range,
// which tmux caps at 15 bytes.
const PRComments = "pr-comments"

// IDs are every action, in the order the footer shows them.
var IDs = []string{PRComments}

// Known reports whether id is an action.
func Known(id string) bool { return slices.Contains(IDs, id) }

// Window options a track's window carries for the footer: how many
// review threads wait on the agent ("" for none), and whether the
// prompt to answer them was sent.
const (
	CommentsOption = "@tracks_pr_comments"
	SentOption     = "@tracks_pr_comments_sent"
)

//go:embed prompts/*.txt
var prompts embed.FS

// Prompt is the line action id types into an agent of engine; false
// when that engine has none. It names the skill or rule that holds the
// steps, so a session started before they were installed finds them.
func Prompt(id, engine string) (string, bool) {
	b, err := prompts.ReadFile("prompts/" + id + "." + engine + ".txt")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// Comments is how a track's open comments stand: Open threads wait on
// the agent, and Sent says the prompt to answer them went out.
type Comments struct {
	Open int
	Sent bool
}

// Next is c once a poll counted open threads. A sent prompt stays sent
// while the count falls, as the agent answers them, and may be sent
// again once new ones come in or none are left.
func (c Comments) Next(open int) Comments {
	return Comments{Open: open, Sent: c.Sent && open > 0 && open <= c.Open}
}
