package hooks

import "github.com/bluegardenproject/tracks/internal/shellx"

// AgentReply is the text of the answer engine's agent just gave, from
// the hook report that carries it: Claude's Stop, Cursor's
// afterAgentResponse. ok is false for every other report.
func AgentReply(engine string, p Payload) (reply string, ok bool) {
	switch {
	case engine == "claude" && p.Event == "Stop":
		return text(p.LastMessage), true
	case engine == "cursor" && p.Event == "afterAgentResponse":
		return text(p.Text), true
	}
	return "", false
}

// ReviewerCommand is the hook command for review run of track id: its
// reports are the reviewer's replies, not the track's status.
func ReviewerCommand(program, engine, id, run string) string {
	return shellx.Quote(program) + " hook --engine " + engine + " --track " + id + " --reviewer " + run
}
