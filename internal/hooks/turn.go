package hooks

// A turn of the agent: it starts as a prompt is sent and ends as the
// agent stops, waiting for the next. The relay holds its pointers back
// while a turn runs.
const (
	TurnStart = "start"
	TurnEnd   = "end"
)

// Turn says whether p, from engine's hook, starts or ends its agent's
// turn: TurnStart, TurnEnd, or "" for neither. A Claude subagent's
// hooks are the main agent's turn going on.
func Turn(engine string, p Payload) string {
	switch {
	case engine == "claude" && p.Agent != "":
		return ""
	case engine == "claude" && p.Event == "UserPromptSubmit", engine == "cursor" && p.Event == "beforeSubmitPrompt":
		return TurnStart
	case engine == "claude" && (p.Event == "Stop" || p.Event == "StopFailure"), engine == "cursor" && p.Event == "stop":
		return TurnEnd
	}
	return ""
}
