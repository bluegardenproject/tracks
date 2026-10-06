package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/shellx"
)

// Timeout is how long an agent waits for a hook, in seconds.
const Timeout = 5

// claudeHooks are the Claude events Tracks subscribes to, each with its
// matcher: a tool name or notification type, "" for all.
var claudeHooks = []struct{ event, matcher string }{
	{"PermissionRequest", ""},
	{"Elicitation", ""},
	{"PreToolUse", "AskUserQuestion|ExitPlanMode"},
	{"Notification", "permission_prompt|elicitation_dialog"},
	{"UserPromptSubmit", ""},
	{"PostToolUse", ""},
	{"PostToolUseFailure", ""},
	{"PermissionDenied", ""},
	{"ElicitationResult", ""},
	{"Stop", ""},
	{"StopFailure", ""},
}

// cursorHooks are the Cursor events Tracks subscribes to. The
// permission hooks (preToolUse, before*Execution) are left alone: a
// wrong answer there blocks the action or skips the user's approval.
var cursorHooks = []string{"beforeSubmitPrompt", "postToolUse", "postToolUseFailure", "stop",
	"afterShellExecution", "afterAgentResponse"}

// Command is the hook command for engine on track id: program is the
// tracks command.
func Command(program, engine, id string) string {
	return shellx.Quote(program) + " hook --engine " + engine + " --track " + id
}

// Install writes the files that give engine's agent on a track its
// hooks into dir, and returns what the agent is started with: Claude's
// settings file, or Cursor's plugin folder. command is the hook
// command; socket is the daemon's, "" for none.
func Install(dir, engine, command, socket string) (string, error) {
	switch engine {
	case "claude":
		path := filepath.Join(dir, "claude-settings.json")
		return path, writeJSON(path, claudeSettings(command, socket))
	case "cursor":
		plugin := filepath.Join(dir, "cursor-plugin")
		if err := writeJSON(filepath.Join(plugin, ".cursor-plugin", "plugin.json"), map[string]string{"name": "tracks-hooks"}); err != nil {
			return "", err
		}
		return plugin, writeJSON(filepath.Join(plugin, "hooks", "hooks.json"), cursorConfig(command))
	}
	return "", fmt.Errorf("no hooks for the engine %s", engine)
}

type claudeHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type claudeMatcher struct {
	Matcher string          `json:"matcher,omitempty"`
	Hooks   []claudeHandler `json:"hooks"`
}

// claudeSettings are the hooks and, with a socket, a sandbox exception
// for it: Claude's sandbox blocks Unix sockets, so the agent's tracks
// commands couldn't reach the daemon.
func claudeSettings(command, socket string) any {
	hooks := map[string][]claudeMatcher{}
	for _, h := range claudeHooks {
		hooks[h.event] = append(hooks[h.event], claudeMatcher{Matcher: h.matcher,
			Hooks: []claudeHandler{{Type: "command", Command: command, Timeout: Timeout}}})
	}
	settings := map[string]any{"hooks": hooks}
	if socket != "" {
		settings["sandbox"] = map[string]any{"network": map[string]any{"allowUnixSockets": []string{socket}}}
	}
	return settings
}

type cursorHandler struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func cursorConfig(command string) any {
	hooks := map[string][]cursorHandler{}
	for _, e := range cursorHooks {
		hooks[e] = []cursorHandler{{Command: command, Timeout: Timeout}}
	}
	return map[string]any{"version": 1, "hooks": hooks}
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
