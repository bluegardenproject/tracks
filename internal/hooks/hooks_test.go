package hooks

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestEvent(t *testing.T) {
	tests := []struct {
		engine, input string
		want          track.Event // "" for none
	}{
		{"claude", `{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"PreToolUse","tool_name":"Bash"}`, ""},
		{"claude", `{"hook_event_name":"Elicitation"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"Notification","notification_type":"idle_prompt"}`, ""},
		{"claude", `{"hook_event_name":"UserPromptSubmit","prompt":"go"}`, track.AgentWorking},
		{"claude", `{"hook_event_name":"PostToolUse","tool_name":"AskUserQuestion"}`, track.AgentWorking},
		{"claude", `{"hook_event_name":"PostToolUse","tool_name":"Bash","agent_id":"a1"}`, ""},
		{"claude", `{"hook_event_name":"PermissionRequest","agent_id":"a1"}`, track.AgentWaiting},
		{"claude", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`, track.AgentWorking},
		{"claude", `{"hook_event_name":"SessionStart"}`, ""},
		{"cursor", `{"hook_event_name":"beforeSubmitPrompt","prompt":"go"}`, track.AgentWorking},
		{"cursor", `{"hook_event_name":"stop","status":"aborted"}`, track.AgentWorking},
		{"cursor", `{"hook_event_name":"afterAgentThought"}`, ""},
		{"codex", `{"hook_event_name":"Stop"}`, ""},
	}
	for _, tt := range tests {
		p, err := Read(strings.NewReader(tt.input))
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Event(tt.engine, p)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%s %s = %q, %v; want %q", tt.engine, tt.input, got, ok, tt.want)
		}
	}
}

func TestReadIsCapped(t *testing.T) {
	big := `{"hook_event_name":"Stop","x":"` + strings.Repeat("a", MaxPayload) + `"}`
	if _, err := Read(strings.NewReader(big)); err == nil {
		t.Error("an oversized payload should fail to decode, not be read whole")
	}
	if _, err := Read(strings.NewReader("not json")); err == nil {
		t.Error("malformed input should be an error")
	}
}

func TestReply(t *testing.T) {
	for _, tt := range []struct{ engine, event, want string }{
		{"claude", "Stop", ""},
		{"cursor", "beforeSubmitPrompt", `{"continue":true}`},
		{"cursor", "stop", "{}"},
	} {
		if got := Reply(tt.engine, tt.event); got != tt.want {
			t.Errorf("Reply(%s, %s) = %q, want %q", tt.engine, tt.event, got, tt.want)
		}
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	command := Command("/bin/tracks dir/tracks", "claude", "20260929-150000-abc123")
	if command != "'/bin/tracks dir/tracks' hook --engine claude --track 20260929-150000-abc123" {
		t.Errorf("command %q", command)
	}

	path, err := Install(dir, "claude", command, "/state/tracks/daemon.sock")
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks   map[string][]claudeMatcher `json:"hooks"`
		Sandbox struct {
			Network struct {
				AllowUnixSockets []string `json:"allowUnixSockets"`
			} `json:"network"`
		} `json:"sandbox"`
	}
	readJSON(t, path, &settings)
	if len(settings.Hooks) != len(claudeHooks) {
		t.Errorf("Claude gets %d hooks, want %d", len(settings.Hooks), len(claudeHooks))
	}
	if got := settings.Sandbox.Network.AllowUnixSockets; len(got) != 1 || got[0] != "/state/tracks/daemon.sock" {
		t.Errorf("Claude's sandbox lets through the sockets %v, want the daemon's", got)
	}
	pre := settings.Hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "AskUserQuestion|ExitPlanMode" || pre[0].Hooks[0].Command != command || pre[0].Hooks[0].Timeout != Timeout {
		t.Errorf("PreToolUse = %+v", pre)
	}

	plugin, err := Install(dir, "cursor", command, "/state/tracks/daemon.sock")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]string
	readJSON(t, filepath.Join(plugin, ".cursor-plugin", "plugin.json"), &manifest)
	if manifest["name"] != "tracks-hooks" {
		t.Errorf("plugin manifest %v", manifest)
	}
	var config struct {
		Version int                        `json:"version"`
		Hooks   map[string][]cursorHandler `json:"hooks"`
	}
	readJSON(t, filepath.Join(plugin, "hooks", "hooks.json"), &config)
	if config.Version != 1 || len(config.Hooks) != len(cursorHooks) || config.Hooks["stop"][0].Command != command {
		t.Errorf("Cursor hooks = %+v", config)
	}
	for _, e := range []string{"preToolUse", "beforeShellExecution", "beforeMCPExecution"} {
		if _, ok := config.Hooks[e]; ok {
			t.Errorf("Cursor's permission hook %s must stay unsubscribed", e)
		}
	}

	if _, err := Install(dir, "codex", command, ""); err == nil {
		t.Error("an engine without hooks should be an error")
	}
	if _, err := os.Stat(filepath.Join(plugin, ".mcp.json")); !os.IsNotExist(err) {
		t.Errorf("Install gives Cursor an MCP server: %v", err)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "mcp__") {
		t.Errorf("Install allows Claude an MCP server: %s", b)
	}
}

func TestInstallAgent(t *testing.T) {
	dir := t.TempDir()
	command := Command("/bin/tracks", "claude", "t1")
	if err := os.MkdirAll(filepath.Join(dir, "cursor-plugin", "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}

	settings, mcpConfig, err := InstallAgent(dir, "claude", command, "/state/daemon.sock", "/state/bin/tracks")
	if err != nil {
		t.Fatal(err)
	}
	var claude struct {
		Hooks       map[string]any `json:"hooks"`
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	readJSON(t, settings, &claude)
	if len(claude.Hooks) != len(claudeHooks) || len(claude.Permissions.Allow) != 1 || claude.Permissions.Allow[0] != "mcp__tracks" {
		t.Errorf("Claude's settings = %+v; want the hooks and mcp__tracks allowed", claude)
	}
	var config struct {
		MCPServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	readJSON(t, mcpConfig, &config)
	if s := config.MCPServers["tracks"]; len(config.MCPServers) != 1 || s.Type != "stdio" || s.Command != "/state/bin/tracks" ||
		len(s.Args) != 1 || s.Args[0] != "mcp" || len(s.Env) != 0 {
		t.Errorf("Claude's MCP config = %+v", config)
	}

	plugin, mcpConfig, err := InstallAgent(dir, "cursor", command, "", "/state/bin/tracks")
	if err != nil || mcpConfig != "" || plugin != filepath.Join(dir, "tracks") {
		t.Fatalf("Cursor: %q, %q, %v; want the plugin folder tracks and no config", plugin, mcpConfig, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor-plugin")); !os.IsNotExist(err) {
		t.Errorf("the old plugin folder is still there: %v", err)
	}
	config.MCPServers = nil
	readJSON(t, filepath.Join(plugin, ".mcp.json"), &config)
	want := map[string]string{"TRACKS_ID": "${env:TRACKS_ID}", "TRACKS_AGENT_TOKEN": "${env:TRACKS_AGENT_TOKEN}",
		"TRACKS_SOCKET_DIR": "${env:TRACKS_SOCKET_DIR}"}
	s := config.MCPServers["tracks"]
	if len(config.MCPServers) != 1 || s.Command != "/state/bin/tracks" || len(s.Args) != 1 || s.Args[0] != "mcp" || !maps.Equal(s.Env, want) {
		t.Errorf("Cursor's .mcp.json = %+v", config)
	}
	if _, err := os.Stat(filepath.Join(plugin, "hooks", "hooks.json")); err != nil {
		t.Errorf("Cursor's hooks: %v", err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
