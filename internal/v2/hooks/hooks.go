// Package hooks is how agent CLIs tell Tracks what they're doing: the
// events each engine's hooks send, what they mean for a track's status,
// and the per-track files that install the hooks. It imports no UI.
package hooks

import (
	"encoding/json"
	"io"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// MaxPayload is how much of a hook's stdin is read.
const MaxPayload = 1 << 20

// Payload is the part of a hook's input Tracks reads. Claude and Cursor
// both name the event in hook_event_name.
type Payload struct {
	Event            string `json:"hook_event_name"`
	Tool             string `json:"tool_name"`
	NotificationType string `json:"notification_type"`
	// Agent is set when the hook fires inside a Claude subagent.
	Agent string `json:"agent_id"`

	// Where PRs are found, kept raw so a field of an unexpected shape
	// costs only the PRs: Claude's tool and last answer, Cursor's shell
	// command and answer.
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	LastMessage  json.RawMessage `json:"last_assistant_message"`
	Command      json.RawMessage `json:"command"`
	Output       json.RawMessage `json:"output"`
	Text         json.RawMessage `json:"text"`
}

// Read decodes a hook's input from r, at most MaxPayload bytes.
func Read(r io.Reader) (Payload, error) {
	var p Payload
	err := json.NewDecoder(io.LimitReader(r, MaxPayload)).Decode(&p)
	return p, err
}

// Event is what p from engine's hook means for the track; false when
// it means nothing.
func Event(engine string, p Payload) (track.Event, bool) {
	switch engine {
	case "claude":
		return claudeEvent(p)
	case "cursor":
		return cursorEvent(p)
	}
	return "", false
}

// claudeEvent maps Claude's hooks: the first ones fire when a dialog
// opens in its window, Notification about six seconds late as the
// backstop.
func claudeEvent(p Payload) (track.Event, bool) {
	switch p.Event {
	case "PermissionRequest", "Elicitation":
		return track.AgentWaiting, true
	case "PreToolUse":
		if p.Tool == "AskUserQuestion" || p.Tool == "ExitPlanMode" {
			return track.AgentWaiting, true
		}
	case "Notification":
		if p.NotificationType == "permission_prompt" || p.NotificationType == "elicitation_dialog" {
			return track.AgentWaiting, true
		}
	case "UserPromptSubmit", "PostToolUse", "PostToolUseFailure", "PermissionDenied", "ElicitationResult", "Stop", "StopFailure":
		// A subagent working on doesn't close a dialog in the window.
		if p.Agent == "" {
			return track.AgentWorking, true
		}
	}
	return "", false
}

// cursorEvent maps Cursor's hooks. None fires when a dialog opens, so
// waiting comes from the pane check.
func cursorEvent(p Payload) (track.Event, bool) {
	switch p.Event {
	case "beforeSubmitPrompt", "postToolUse", "postToolUseFailure", "stop":
		return track.AgentWorking, true
	}
	return "", false
}

// Reply is what engine's hook prints for event so the agent carries on
// as if it had no hook.
func Reply(engine, event string) string {
	switch {
	case engine != "cursor":
		return ""
	case event == "beforeSubmitPrompt":
		return `{"continue":true}`
	}
	return "{}"
}
