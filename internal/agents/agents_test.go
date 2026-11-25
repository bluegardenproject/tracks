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

func TestVersion(t *testing.T) {
	for out, want := range map[string]string{
		"2.1.281 (Claude Code)\n":                                    "2.1.281",
		"GitHub Copilot CLI 1.0.114.\nRun 'copilot update' to check": "1.0.114",
		"2026.11.20-1a2b3c\n":                                        "2026.11.20-1a2b3c",
		"some tool, no number":                                       "some tool, no number",
	} {
		if got := version(out); got != want {
			t.Errorf("version(%q) = %q, want %q", out, got, want)
		}
	}
}

func TestRunnersLeaveOutPreviews(t *testing.T) {
	for _, e := range Runners() {
		if e.Preview {
			t.Errorf("%s is a preview but runs tracks", e.Name)
		}
	}
	if len(Runners()) != len(All)-1 {
		t.Errorf("Runners = %d engines, want all %d but Copilot", len(Runners()), len(All))
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
	mcpDir := filepath.Join(t.TempDir(), "mcp")
	if got, err := ListMCP(context.Background(), path, mcpDir); err != nil || len(got) != 1 || got[0].Name != "linear" {
		t.Errorf("ListMCP = %+v, %v; want linear", got, err)
	}
	failing := fakePath(t, "agent", `echo "Failed to list MCP servers: no network"; exit 1`)
	if _, err := ListMCP(context.Background(), failing, mcpDir); err == nil || !strings.Contains(err.Error(), "no network") {
		t.Errorf("a failing list should say why, got %v", err)
	}

	// Explicit dir is created 0700 and reused across calls to avoid per-project config churn.
	cwdOut := filepath.Join(t.TempDir(), "cwd.txt")
	t.Setenv("TEST_MCP_CWD_OUT", cwdOut)
	pwdCLI := fakePath(t, "agent", `pwd > "$TEST_MCP_CWD_OUT"; echo "linear: ready"`)
	if _, err := ListMCP(context.Background(), pwdCLI, mcpDir); err != nil {
		t.Fatalf("ListMCP stable dir (1): %v", err)
	}
	cwd1, err := os.ReadFile(cwdOut)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListMCP(context.Background(), pwdCLI, mcpDir); err != nil {
		t.Fatalf("ListMCP stable dir (2): %v", err)
	}
	cwd2, err := os.ReadFile(cwdOut)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(cwd1)) != strings.TrimSpace(string(cwd2)) {
		t.Errorf("ListMCP changed cwd across calls: %q vs %q", cwd1, cwd2)
	}
	if info, err := os.Lstat(mcpDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("mcpDir stat = %v, %v; want 0700 dir", info, err)
	}
	// A symlink at dir is refused.
	badLink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), badLink); err != nil {
		t.Fatal(err)
	}
	if _, err := ListMCP(context.Background(), path, badLink); err == nil {
		t.Error("ListMCP accepted symlink as working dir")
	}

	// Empty dir falls back to a private temp dir (never shared os.TempDir()) and cleans it up.
	if _, err := ListMCP(context.Background(), pwdCLI, ""); err != nil {
		t.Fatalf("ListMCP fallback temp dir: %v", err)
	}
	cwdBytes, err := os.ReadFile(cwdOut)
	if err != nil {
		t.Fatal(err)
	}
	cwd := strings.TrimSpace(string(cwdBytes))
	tmpReal, _ := filepath.EvalSymlinks(os.TempDir())
	if cwd == filepath.Clean(os.TempDir()) || cwd == tmpReal {
		t.Errorf("ListMCP ran directly in shared temp dir %q", cwd)
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Errorf("ListMCP temp dir %q was not cleaned up", cwd)
	}

	// Cancelled context returns a readable message instead of bare "context canceled".
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListMCP(cancelled, path, mcpDir); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("ListMCP(cancelled) = %v; want readable cancellation message", err)
	}
}

func TestMCPState(t *testing.T) {
	for status, want := range map[string]MCPState{
		"ready":                   MCPReady,
		"✔ Connected":             MCPReady,
		"connected":               MCPReady,
		"disconnected":            MCPAttention,
		"Disconnected":            MCPAttention,
		"not connected":           MCPAttention,
		"not-connected":           MCPAttention,
		"requires_authentication": MCPAttention,
		"Failed to connect":       MCPFailed,
		"Error: timeout":          MCPFailed,
	} {
		if got := mcpState(status); got != want {
			t.Errorf("mcpState(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestWriteAndRemoveManagedHardening(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "target.md")
	managed := []byte("---\nx-tracks-managed: \"1\"\n---\nbody\n")

	// Dangling symlink must neither be overwritten nor create its target.
	dangling := filepath.Join(dir, "dangling.md")
	if err := os.Symlink(outside, dangling); err != nil {
		t.Fatal(err)
	}
	if ok, err := WriteManaged(dangling, managed); err != nil || ok {
		t.Fatalf("WriteManaged(dangling symlink) = %v, %v; want false, nil", ok, err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Errorf("WriteManaged created target through dangling symlink")
	}

	// Symlink to a file containing the marker must not be overwritten or removed.
	if err := os.WriteFile(outside, managed, 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := WriteManaged(dangling, append(managed, 'x')); err != nil || ok {
		t.Fatalf("WriteManaged(symlink to managed) = %v, %v; want false, nil", ok, err)
	}
	if err := RemoveManaged(dangling); err != nil {
		t.Fatalf("RemoveManaged(symlink): %v", err)
	}
	if _, err := os.Lstat(dangling); err != nil {
		t.Errorf("RemoveManaged deleted symlink: %v", err)
	}

	// Prose mentioning the marker outside frontmatter, indented lines,
	// or false values must not be treated as managed.
	for name, body := range map[string]string{
		"prose.md":         "This doc mentions x-tracks-managed: \"1\" in prose.\n",
		"after-fm.md":      "---\nname: user\n---\nx-tracks-managed: \"1\"\n",
		"indented.md":      "---\ndescription: |\n  x-tracks-managed: \"1\"\n---\nuser content\n",
		"false-flag.md":    "---\nx-tracks-managed: false\n---\nuser content\n",
		"false-comment.md": "---\nx-tracks-managed: false # keep\n---\nuser content\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if ok, err := WriteManaged(p, managed); err != nil || ok {
			t.Errorf("WriteManaged(%s) = %v, %v; want false, nil", name, ok, err)
		}
		if err := RemoveManaged(p); err != nil {
			t.Errorf("RemoveManaged(%s): %v", name, err)
		}
		if got, _ := os.ReadFile(p); string(got) != body {
			t.Errorf("%s was modified or removed", name)
		}
	}

	// Regular managed file is atomically updated (preserving its mode) and removed.
	own := filepath.Join(dir, "managed.md")
	if ok, err := WriteManaged(own, managed); err != nil || !ok {
		t.Fatalf("WriteManaged(new) = %v, %v; want true, nil", ok, err)
	}
	if info, err := os.Lstat(own); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("new managed file mode = %v, %v; want 0644", info, err)
	}
	if err := os.Chmod(own, 0o600); err != nil {
		t.Fatal(err)
	}
	updated := []byte("---\nx-tracks-managed: \"2\"\n---\nupdated body\n")
	if ok, err := WriteManaged(own, updated); err != nil || !ok {
		t.Fatalf("WriteManaged(update) = %v, %v; want true, nil", ok, err)
	}
	if got, err := os.ReadFile(own); err != nil || string(got) != string(updated) {
		t.Fatalf("after update: %q, %v; want %q", got, err, updated)
	}
	if info, err := os.Lstat(own); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("updated managed file mode = %v, %v; want preserved 0600", info, err)
	}
	if err := RemoveManaged(own); err != nil {
		t.Fatalf("RemoveManaged(managed): %v", err)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Errorf("RemoveManaged did not remove managed file")
	}
}
