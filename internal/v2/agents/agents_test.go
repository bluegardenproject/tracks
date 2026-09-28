package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real output from `agent --list-models`, warts included: a header
// line, a "(current, default)" suffix, zero-width spaces in labels, and the
// certificate warnings the CLI prints on macOS.
const realListModels = `ERROR: failed to copy trust settings of system certificate-25291
Available models

auto - Auto (current, default)
gpt-5.3-codex-low - Codex 5.3 Low
claude-opus-5-thinking-high - Claude Opus 5 1M Thinking
cursor-grok-4.6-low-fast - Cursor Grok 4.6 Low Fast` + "\u200b\u200b" + `
composer-2.5 - Composer 2.5
`

func TestParseModelList(t *testing.T) {
	got := parseModelList(realListModels)
	if len(got) != 4 || got[0] != (Model{"gpt-5.3-codex-low", "Codex 5.3 Low"}) {
		t.Fatalf("parsed %+v; want the four models after auto", got)
	}
	if got[2].Label != "Cursor Grok 4.6 Low Fast" {
		t.Errorf("label %q; want the zero-width spaces gone", got[2].Label)
	}
	for name, in := range map[string]string{
		"header only":  "Available models\n",
		"prose dash":   "Please run agent login - then try again\n",
		"empty":        "",
		"no separator": "gpt-5.3-codex\n",
	} {
		if got := parseModelList(in); len(got) != 0 {
			t.Errorf("%s: parsed %+v", name, got)
		}
	}
}

// fakePath puts a program called name, running script, on a PATH of its own.
func fakePath(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestCheck(t *testing.T) {
	path := fakePath(t, "claude", `echo "2.1.281 (Claude Code)"`)
	got, err := Check(context.Background(), Claude)
	if err != nil || got.Path != path || got.Version != "2.1.281" {
		t.Errorf("Check = %+v, %v; want %s at 2.1.281", got, err, path)
	}
	if _, err := Check(context.Background(), Cursor); !errors.Is(err, ErrNotFound) {
		t.Errorf("Check of a missing CLI = %v; want ErrNotFound", err)
	}
}

func TestListModels(t *testing.T) {
	path := fakePath(t, "agent", "/bin/cat <<'OUT'\n"+realListModels+"OUT")
	if got, err := ListModels(context.Background(), path); err != nil || len(got) != 4 {
		t.Errorf("ListModels = %v, %v; want four models", got, err)
	}
	empty := fakePath(t, "agent", "echo 'Please log in'")
	if _, err := ListModels(context.Background(), empty); err == nil {
		t.Error("an empty list should be an error")
	}
}

func TestParseMCPList(t *testing.T) {
	claude := "Checking MCP server health…\n\nfigma: https://mcp.figma.com/mcp (HTTP) - ✔ Connected\n" +
		"context7: npx -y @upstash/context7-mcp - ✘ Failed to connect\nlocal: ./run.sh - ⏸ Pending approval (run `claude` to approve)\n"
	got := parseMCPList(claude)
	want := []MCPServer{
		{"figma", "https://mcp.figma.com/mcp (HTTP)", "✔ Connected", MCPReady},
		{"context7", "npx -y @upstash/context7-mcp", "✘ Failed to connect", MCPFailed},
		{"local", "./run.sh", "⏸ Pending approval (run `claude` to approve)", MCPAttention},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Claude Code's list parsed as\n%v\nwant\n%v", got, want)
	}

	cursor := "\x1b[32mcontext7: Error: Connection failed\x1b[39m\nlinear: \x1b[32mready\x1b[39m\nsentry: requires_authentication\n"
	got = parseMCPList(cursor)
	if len(got) != 3 || got[0].Status != "Error: Connection failed" || got[0].State != MCPFailed ||
		got[1] != (MCPServer{Name: "linear", Status: "ready"}) || got[2].State != MCPAttention {
		t.Errorf("Cursor's list parsed as %+v", got)
	}
	for _, none := range []string{"No MCP servers configured. Use `claude mcp add` to add a server.\n",
		"No MCP servers configured (expected in .cursor/mcp.json or ~/.cursor/mcp.json)\n"} {
		if got := parseMCPList(none); len(got) != 0 {
			t.Errorf("%q parsed as %+v", none, got)
		}
	}
}

func TestListMCP(t *testing.T) {
	path := fakePath(t, "agent", `[ "$1 $2" = "mcp list" ] && echo "linear: ready"`)
	if got, err := ListMCP(context.Background(), path); err != nil || len(got) != 1 || got[0].Name != "linear" {
		t.Errorf("ListMCP = %+v, %v; want linear", got, err)
	}
	failing := fakePath(t, "agent", `echo "Failed to list MCP servers: no network"; exit 1`)
	if _, err := ListMCP(context.Background(), failing); err == nil || !strings.Contains(err.Error(), "no network") {
		t.Errorf("a failing list should say why, got %v", err)
	}
}
