package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestMCPTools(t *testing.T) {
	if got := mcpTools(mcpEnv{Track: "t1"}, "/s/daemon.sock", "tracks"); len(got) != 0 {
		t.Errorf("without a token: %d tools, want none", len(got))
	}
	if got := mcpTools(mcpEnv{Token: "tok", Track: "t1", Reviewer: true}, "/s/daemon.sock", "tracks"); len(got) != 0 {
		t.Errorf("a reviewer: %d tools, want none", len(got))
	}
	var names []string
	for _, tool := range mcpTools(mcpEnv{Token: "tok"}, "/s/daemon.sock", "tracks") {
		if tool.Description == "" || tool.Schema["type"] != "object" {
			t.Errorf("%s: description %q, schema %v", tool.Name, tool.Description, tool.Schema)
		}
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, " "); got != "relay_send relay_read relay_wait relay_who proxy_status panes_list" {
		t.Errorf("tools %s", got)
	}
	if got, err := mcpSocket("/state/tracks"); err != nil || got != "/state/tracks/daemon.sock" {
		t.Errorf("mcpSocket = %q, %v", got, err)
	}
}

// TestMCPEndToEnd runs tracks mcp on stdio against a daemon whose files
// are in a temporary HOME and XDG_STATE_HOME: a side track's agent
// sends its main track a message, which the main track's agent waits
// for and reads.
func TestMCPEndToEnd(t *testing.T) {
	ctx := context.Background()
	paths := tempTracks(t)
	st, err := store.Open(ctx, paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	main := track.Track{ID: "20261127-100000-main01", Kind: track.Work, Name: "main", Engine: "claude", CreatedAt: time.Now()}
	side := track.Track{ID: "20261127-100000-side01", Kind: track.Work, Name: "side", Engine: "cursor", CreatedAt: time.Now(),
		Parent: main.ID, Side: "side-1"}
	tokens := map[string]string{}
	for _, tr := range []track.Track{main, side} {
		if err := st.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
		tokens[tr.ID] = agents.NewToken()
		if err := st.SetAgentToken(ctx, tr.ID, agents.HashToken(tokens[tr.ID])); err != nil {
			t.Fatal(err)
		}
	}
	runDaemon(t, paths, st)

	// Without a token, the server offers nothing.
	none := startMCP(t, "", "", "")
	if tools := none.call("tools/list", nil)["tools"].([]any); len(tools) != 0 {
		t.Errorf("tools without a token: %v", tools)
	}

	// The main track's agent finds the daemon where Tracks keeps it.
	mainAgent := startMCP(t, main.ID, tokens[main.ID], "")
	if tools := mainAgent.call("tools/list", nil)["tools"].([]any); len(tools) != 6 {
		t.Errorf("tools/list = %v, want 6 tools", tools)
	}
	who, isErr := mainAgent.tool("relay_who", nil)
	if isErr || !strings.Contains(who, "@track") || !strings.Contains(who, "@side-1") {
		t.Errorf("relay_who = %q, error %v; want the track and its side track", who, isErr)
	}
	// What tracks servers says, here a daemon without tmux's error.
	got, isErr := mainAgent.tool("proxy_status", nil)
	var cliOut strings.Builder
	root := newRoot("v-test")
	root.SetArgs([]string{"servers"})
	root.SetOut(&cliOut)
	if cliErr := root.ExecuteContext(ctx); (cliErr != nil) != isErr || (cliErr != nil && got != cliErr.Error()) ||
		(cliErr == nil && got != cliOut.String()) {
		t.Errorf("proxy_status = %q, error %v; want tracks servers' %q, %v", got, isErr, cliOut.String(), cliErr)
	}
	// The daemon's notices from its start, then nothing new.
	if got, isErr := mainAgent.tool("relay_read", nil); isErr {
		t.Errorf("relay_read = %q, error %v", got, isErr)
	}
	if got, isErr := mainAgent.tool("relay_read", nil); isErr || got != "No new messages." {
		t.Errorf("relay_read again = %q, error %v", got, isErr)
	}
	waiting := mainAgent.start("tools/call", map[string]any{"name": "relay_wait"})

	// The side track's agent finds it through TRACKS_SOCKET_DIR, as
	// Cursor's config passes it, and names a track that isn't its own.
	sideAgent := startMCP(t, main.ID, tokens[side.ID], paths.DataDir)
	got, isErr = sideAgent.tool("relay_send", map[string]any{"to": "track", "kind": "report", "body": "Done: #12.",
		"outcome": "pass", "refs": []string{"https://github.com/acme/api/pull/12"}})
	if isErr || !strings.HasPrefix(got, "Sent #") || !strings.HasSuffix(got, " to @track.") {
		t.Errorf("relay_send = %q, error %v", got, isErr)
	}
	text, isErr := toolText(t, mainAgent.answer(waiting))
	if isErr || !strings.HasPrefix(text, "Messages on this track's relay.") || !strings.Contains(text, "@side-1 → @track · report") ||
		!strings.Contains(text, "Done: #12.") || !strings.Contains(text, "pass") || !strings.Contains(text, "https://github.com/acme/api/pull/12") {
		t.Errorf("relay_wait = %q, error %v; want the side track's report", text, isErr)
	}

	// Its mistakes are told in the CLI's words.
	mcpErr, isErr := sideAgent.tool("relay_send", map[string]any{"to": "track", "kind": "report", "body": "Hi", "outcome": "maybe"})
	t.Setenv("TRACKS_ID", side.ID)
	root = newRoot("v-test")
	root.SetArgs([]string{"relay", "send", "--to", "track", "--kind", "report", "--outcome", "maybe"})
	root.SetIn(strings.NewReader("Hi"))
	root.SetOut(io.Discard)
	cliErr := root.ExecuteContext(ctx)
	if !isErr || cliErr == nil || mcpErr != cliErr.Error() {
		t.Errorf("relay_send's error %q, error %v; want the CLI's, %v", mcpErr, isErr, cliErr)
	}

	stranger := startMCP(t, main.ID, agents.NewToken(), "")
	if got, isErr := stranger.tool("relay_read", nil); !isErr || !strings.Contains(got, "doesn't know this agent's token") {
		t.Errorf("an unknown token's relay_read = %q, error %v", got, isErr)
	}

	// The panes are read from the track's window, on a throwaway tmux
	// server here, which has none.
	tools := mcpTools(mcpEnv{Token: tokens[main.ID], Track: main.ID}, paths.Socket, tmuxtest.Socket(t))
	panes := tools[len(tools)-1]
	if _, err := panes.Call(ctx, json.RawMessage("{}")); err == nil || !strings.HasPrefix(err.Error(), "list the panes:") {
		t.Errorf("panes_list without a window: %v, want it failing on the window", err)
	}
}

// tempTracks points HOME and the XDG folders at a short temporary
// folder, a Unix socket's path having to stay under about 100 bytes,
// and returns Tracks' paths there.
func tempTracks(t *testing.T) platform.Paths {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tmcp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv(review.EnvMarker, "")
	paths, err := platform.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return paths
}

// runDaemon runs a daemon on paths and st until the test ends, without
// tmux.
func runDaemon(t *testing.T, paths platform.Paths, st *store.Store) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- daemon.Run(context.Background(), daemon.Config{
			Paths: paths, Version: "v-test", Session: sessionName, Tmux: liveTmux{}, Home: os.Getenv("HOME"),
			Log: log.New(io.Discard, "", 0), Every: time.Hour, PollEvery: time.Hour,
			Tracks: &tracks.Service{Store: st, Windows: noWindows{}, SocketDir: paths.DataDir, BinDir: paths.BinDir,
				Socket: paths.Socket, Settings: func() (settings.Settings, error) { return settings.Settings{}, nil }},
		})
	}()
	client := rpc.Client{Socket: paths.Socket}
	t.Cleanup(func() {
		_ = client.Shutdown(context.Background())
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the daemon didn't exit")
		}
	})
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := client.Ping(context.Background()); err == nil {
			return
		}
	}
	t.Fatal("the daemon didn't answer")
}

type liveTmux struct{}

func (liveTmux) HasSession(string) bool    { return true }
func (liveTmux) ServerPID() (int, error)   { return 1, nil }
func (liveTmux) Tell(string, string) error { return nil }

type noWindows struct{}

func (noWindows) List() ([]trackwin.Info, error)        { return nil, nil }
func (noWindows) LastKey(string) (time.Time, error)     { return time.Time{}, nil }
func (noWindows) StyledScreen(string) (string, error)   { return "", nil }
func (noWindows) Paste(string, string) error            { return nil }
func (noWindows) Scrolled(string) (bool, error)         { return false, nil }
func (noWindows) LeaveScroll(string) error              { return nil }
func (noWindows) Platforms() (map[string]string, error) { return nil, nil }
func (noWindows) OpenPlatform(string, string) error     { return nil }
func (noWindows) FitPlatforms() error                   { return nil }
func (noWindows) Open(trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Window{}, errors.New("no windows")
}
func (noWindows) Close(string) error     { return nil }
func (noWindows) EndHidden(string) error { return nil }
func (noWindows) Respawn(string, trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Window{}, errors.New("no windows")
}
func (noWindows) Attention(string, bool) error  { return nil }
func (noWindows) Screen(string) (string, error) { return "", nil }
func (noWindows) Buttons(string, string) error  { return nil }
func (noWindows) Prompt(string, string) error   { return nil }
func (noWindows) Info(string, string) error     { return nil }

// mcpClient talks to a tracks mcp over its stdin and stdout.
type mcpClient struct {
	t       *testing.T
	in      io.Writer
	answers chan map[string]any
	pending map[float64]map[string]any
	nextID  float64
}

// startMCP runs tracks mcp with the agent's TRACKS_ID, token and
// TRACKS_SOCKET_DIR, and initializes it.
func startMCP(t *testing.T, id, token, socketDir string) *mcpClient {
	t.Helper()
	t.Setenv("TRACKS_ID", id)
	t.Setenv(agents.TokenEnv, token)
	t.Setenv("TRACKS_SOCKET_DIR", socketDir)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	root := newRoot("v-test")
	root.SetArgs([]string{"mcp"})
	root.SetIn(inR)
	root.SetOut(outW)
	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(context.Background())
		outW.Close()
	}()
	c := &mcpClient{t: t, in: inW, answers: make(chan map[string]any, 16), pending: map[float64]map[string]any{}}
	go func() {
		scan := bufio.NewScanner(outR)
		scan.Buffer(nil, 1<<20)
		for scan.Scan() {
			var m map[string]any
			if err := json.Unmarshal(scan.Bytes(), &m); err == nil {
				c.answers <- m
			}
		}
		close(c.answers)
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("tracks mcp: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("tracks mcp didn't exit once its stdin closed")
		}
	})
	init := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"}})
	if init["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize = %v", init)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c
}

func (c *mcpClient) send(msg map[string]any) {
	c.t.Helper()
	b, _ := json.Marshal(msg)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// start sends a request and returns its id, for answer.
func (c *mcpClient) start(method string, params any) float64 {
	c.t.Helper()
	c.nextID++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	c.send(msg)
	return c.nextID
}

// answer is the answer to request id; the others that come first wait
// for theirs.
func (c *mcpClient) answer(id float64) map[string]any {
	c.t.Helper()
	for {
		if m, ok := c.pending[id]; ok {
			delete(c.pending, id)
			return m
		}
		select {
		case m, ok := <-c.answers:
			if !ok {
				c.t.Fatal("tracks mcp closed its output")
			}
			c.pending[m["id"].(float64)] = m
		case <-time.After(10 * time.Second):
			c.t.Fatalf("no answer to %v", id)
		}
	}
}

// call is the result of a request.
func (c *mcpClient) call(method string, params any) map[string]any {
	c.t.Helper()
	m := c.answer(c.start(method, params))
	result, ok := m["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("%s: %v", method, m)
	}
	return result
}

// tool calls a tool and returns its text and whether it's an error.
func (c *mcpClient) tool(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	return toolText(c.t, c.answer(c.start("tools/call", params)))
}

func toolText(t *testing.T, answer map[string]any) (string, bool) {
	t.Helper()
	result, ok := answer["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", answer)
	}
	content := result["content"].([]any)
	return content[0].(map[string]any)["text"].(string), result["isError"].(bool)
}
