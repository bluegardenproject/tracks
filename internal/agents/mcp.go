package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// MCPServer is an MCP server an engine is set up with, and how the CLI
// found it.
type MCPServer struct {
	Name   string
	Detail string // what it runs or where it is, when the CLI says
	Status string // in the CLI's words
	State  MCPState
}

// MCPState sorts a server's status.
type MCPState int

const (
	MCPReady     MCPState = iota
	MCPAttention          // pending approval, needs a login, disabled, loading
	MCPFailed
)

// Both CLIs connect to every server to report on it.
const mcpTimeout = time.Minute

// ListMCP asks the CLI at path which MCP servers it's set up with. It
// runs outside any repo, so only servers set up for every project are
// listed; a repo can add its own.
func ListMCP(ctx context.Context, path string) ([]MCPServer, error) {
	ctx, cancel := context.WithTimeout(ctx, mcpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "mcp", "list")
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, errors.New("checking the MCP servers took too long")
	}
	if err != nil {
		return nil, fmt.Errorf("%s mcp list: %s", path, lastLine(string(out), err))
	}
	return parseMCPList(string(out)), nil
}

// parseMCPList reads the `name: status` lines Cursor prints and the
// `name: target - status` lines Claude Code prints, skipping the rest.
func parseMCPList(out string) []MCPServer {
	var servers []MCPServer
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(clean(line)), ": ")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		detail, status := "", strings.TrimSpace(rest)
		if i := strings.LastIndex(rest, " - "); i >= 0 {
			detail, status = strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+3:])
		}
		servers = append(servers, MCPServer{Name: name, Detail: detail, Status: status, State: mcpState(status)})
	}
	return servers
}

func mcpState(status string) MCPState {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "fail") || strings.Contains(s, "error") || strings.Contains(s, "rejected"):
		return MCPFailed
	case s == "ready" || strings.Contains(s, "connected"):
		return MCPReady
	}
	return MCPAttention
}

// lastLine is the output's last line, or err when there's none.
func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if last := strings.TrimSpace(clean(lines[len(lines)-1])); last != "" {
		return last
	}
	return err.Error()
}
