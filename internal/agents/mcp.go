package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// runs outside any repo, in dir (kept as a private 0700 folder when set,
// or a throwaway private temp folder when empty), so only servers set
// up for every project are listed; a repo can add its own.
func ListMCP(ctx context.Context, path, dir string) ([]MCPServer, error) {
	ctx, cancel := context.WithTimeout(ctx, mcpTimeout)
	defer cancel()
	workDir, cleanup, err := prepareMCPDir(dir)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	cmd := exec.CommandContext(ctx, path, "mcp", "list")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("checking the MCP servers took too long")
	}
	if ctx.Err() != nil {
		return nil, errors.New("checking the MCP servers was cancelled")
	}
	if err != nil {
		return nil, fmt.Errorf("%s mcp list: %s", path, lastLine(string(out), err))
	}
	return parseMCPList(string(out)), nil
}

func prepareMCPDir(dir string) (string, func(), error) {
	if dir == "" {
		tmp, err := os.MkdirTemp("", "tracks-mcp-*")
		if err != nil {
			return "", nil, err
		}
		return tmp, func() { _ = os.RemoveAll(tmp) }, nil
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("mcp working directory %s is not a directory", dir)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return "", nil, err
		}
	}
	return dir, func() {}, nil
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
	case strings.Contains(s, "disconnect") || strings.Contains(s, "not connected"):
		return MCPAttention
	case s == "ready" || hasWord(s, "connected"):
		return MCPReady
	}
	return MCPAttention
}

func hasWord(s, word string) bool {
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], word)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(word)
		leftOK := start == 0 || !isWordChar(s[start-1])
		rightOK := end == len(s) || !isWordChar(s[end])
		if leftOK && rightOK {
			return true
		}
		i = end
	}
	return false
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

// lastLine is the output's last line, or err when there's none.
func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if last := strings.TrimSpace(clean(lines[len(lines)-1])); last != "" {
		return last
	}
	return err.Error()
}
