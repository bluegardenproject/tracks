package hooks

import (
	"os"
	"path/filepath"
)

// MCPServer is the name a track's agent knows Tracks' MCP server by.
const MCPServer = "tracks"

// InstallAgent writes what a track's agent starts with into dir: its
// hooks, as Install writes them, and Tracks' MCP server, which program
// runs as `program mcp`. It returns the hooks as the engine takes them
// and, for Claude, its --mcp-config file; Cursor's server is in the
// plugin. The reviewer gets Install's hooks only, and no server.
func InstallAgent(dir, engine, command, socket, program string) (hooks, mcpConfig string, err error) {
	if hooks, err = install(dir, engine, command, socket, []string{"mcp__" + MCPServer}); err != nil {
		return "", "", err
	}
	switch engine {
	case "claude":
		mcpConfig = filepath.Join(dir, "claude-mcp.json")
		err = writeJSON(mcpConfig, claudeMCP(program))
	case "cursor":
		err = writeJSON(filepath.Join(hooks, ".mcp.json"), cursorMCP(program))
	}
	if err != nil {
		return "", "", err
	}
	return hooks, mcpConfig, nil
}

// claudeMCP is Claude's --mcp-config: the server inherits the pane's
// environment, its TRACKS_ID and TRACKS_AGENT_TOKEN.
func claudeMCP(program string) any {
	return map[string]any{"mcpServers": map[string]any{
		MCPServer: map[string]any{"type": "stdio", "command": program, "args": []string{"mcp"}},
	}}
}

// cursorMCP is the plugin's .mcp.json. Cursor gives a server little of
// its environment, so the config passes on what tracks mcp reads.
func cursorMCP(program string) any {
	env := map[string]string{}
	for _, name := range []string{"TRACKS_ID", "TRACKS_AGENT_TOKEN", "TRACKS_SOCKET_DIR"} {
		env[name] = "${env:" + name + "}"
	}
	return map[string]any{"mcpServers": map[string]any{
		MCPServer: map[string]any{"command": program, "args": []string{"mcp"}, "env": env},
	}}
}

// removeLegacyPlugin removes the plugin folder Cursor's hooks were in
// before it was named after Tracks' server: plugin-tracks-tracks.
func removeLegacyPlugin(dir string) { _ = os.RemoveAll(filepath.Join(dir, "cursor-plugin")) }
