package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestNegotiate(t *testing.T) {
	for asked, want := range map[string]string{
		"2025-06-18": "2025-06-18",
		"2024-11-05": "2024-11-05",
		"2099-01-01": Versions[0],
		"":           Versions[0],
	} {
		if got := Negotiate(asked); got != want {
			t.Errorf("Negotiate(%q) = %q, want %q", asked, got, want)
		}
	}
}

// session runs s on pipes and returns a function that sends a line and
// one that reads the next answer.
func session(t *testing.T, s Server) (send func(string), next func() map[string]any, closeIn func()) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- s.Serve(context.Background(), inR, outW)
		outW.Close()
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Serve didn't return once stdin closed")
		}
	})
	lines := make(chan string)
	go func() {
		scan := bufio.NewScanner(outR)
		for scan.Scan() {
			lines <- scan.Text()
		}
		close(lines)
	}()
	send = func(line string) {
		t.Helper()
		if _, err := io.WriteString(inW, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	next = func() map[string]any {
		t.Helper()
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("the server closed its output")
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("no answer")
			return nil
		}
	}
	return send, next, func() { inW.Close() }
}

func text(t *testing.T, answer map[string]any) (string, bool) {
	t.Helper()
	result, ok := answer["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", answer)
	}
	content := result["content"].([]any)
	return content[0].(map[string]any)["text"].(string), result["isError"].(bool)
}

func TestServe(t *testing.T) {
	release := make(chan struct{})
	waited := make(chan error, 1)
	s := Server{Name: "tracks", Version: "v-test", Tools: []Tool{
		{Name: "echo", Description: "Says it back.", Schema: map[string]any{"type": "object"},
			Call: func(_ context.Context, args json.RawMessage) (string, error) {
				var a struct{ Say string }
				err := json.Unmarshal(args, &a)
				return a.Say, err
			}},
		{Name: "fail", Schema: map[string]any{"type": "object"},
			Call: func(context.Context, json.RawMessage) (string, error) { return "", errors.New("no relay") }},
		{Name: "wait", Schema: map[string]any{"type": "object"},
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				select {
				case <-ctx.Done():
					waited <- ctx.Err()
					return "", ctx.Err()
				case <-release:
					return "done", nil
				}
			}},
	}}
	send, next, _ := session(t, s)

	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	init := next()
	result := init["result"].(map[string]any)
	if init["id"].(float64) != 1 || result["protocolVersion"] != "2025-06-18" || result["serverInfo"].(map[string]any)["name"] != "tracks" {
		t.Errorf("initialize = %v", init)
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("no tools capability: %v", result)
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	send(`{"jsonrpc":"2.0","id":"l","method":"tools/list"}`)
	list := next()
	tools := list["result"].(map[string]any)["tools"].([]any)
	if list["id"] != "l" || len(tools) != 3 || tools[0].(map[string]any)["name"] != "echo" ||
		tools[0].(map[string]any)["description"] != "Says it back." || tools[0].(map[string]any)["inputSchema"] == nil {
		t.Errorf("tools/list = %v", list)
	}

	// A slow call doesn't hold up the next ones.
	send(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"wait"}}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hi"}}}`)
	if got, isErr := text(t, next()); got != "hi" || isErr {
		t.Errorf("echo = %q, error %v", got, isErr)
	}
	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"fail","arguments":{}}}`)
	if got, isErr := text(t, next()); got != "no relay" || !isErr {
		t.Errorf("fail = %q, error %v; want the error as the tool's result", got, isErr)
	}
	send(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`)
	if e, ok := next()["error"].(map[string]any); !ok || e["code"].(float64) != codeInvalidParams {
		t.Errorf("an unknown tool: %v", e)
	}
	send(`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`)
	if e, ok := next()["error"].(map[string]any); !ok || e["code"].(float64) != codeNoMethod {
		t.Errorf("an unknown method: %v", e)
	}
	send(`not json`)
	if e, ok := next()["error"].(map[string]any); !ok || e["code"].(float64) != codeParse {
		t.Errorf("a broken line: %v", e)
	}
	send(`{"jsonrpc":"2.0","id":6,"method":"ping"}`)
	if ping := next(); ping["id"].(float64) != 6 || ping["result"] == nil {
		t.Errorf("ping = %v", ping)
	}

	send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":9}}`)
	select {
	case err := <-waited:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the cancelled call ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled call goes on")
	}
	// No answer to the cancelled call: the next one is ping's.
	send(`{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if ping := next(); ping["id"].(float64) != 7 {
		t.Errorf("after the cancel: %v, want ping's answer", ping)
	}
}

func TestServeWithoutTools(t *testing.T) {
	send, next, _ := session(t, Server{Name: "tracks"})
	send(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	answer := next()
	if tools, ok := answer["result"].(map[string]any)["tools"].([]any); !ok || len(tools) != 0 {
		t.Errorf("tools/list = %v, want an empty list", answer)
	}
}

func TestServeCancelsCallsWhenStdinCloses(t *testing.T) {
	ended := make(chan struct{})
	s := Server{Tools: []Tool{{Name: "wait", Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
		<-ctx.Done()
		close(ended)
		return "", ctx.Err()
	}}}}
	send, _, closeIn := session(t, s)
	send(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"wait"}}`)
	time.Sleep(20 * time.Millisecond)
	closeIn()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the call goes on after stdin closed")
	}
}
