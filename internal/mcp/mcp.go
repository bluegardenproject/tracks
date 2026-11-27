// Package mcp is a Model Context Protocol server over stdio, as much of
// it as tracks mcp needs: tools, and nothing else. Each message is a
// JSON-RPC 2.0 object on a line of its own.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync"
)

// Versions are the protocol versions the server speaks, newest first.
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is a tool the server offers. Call answers a call with text, or
// an error the agent reads as the tool's result.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON Schema of the arguments, an object.
	Schema map[string]any
	Call   func(ctx context.Context, args json.RawMessage) (string, error)
}

// Server answers on stdio with its tools.
type Server struct {
	Name, Version string
	// Instructions, when set, tell the agent what the tools are for.
	Instructions string
	Tools        []Tool
}

// maxLine bounds a message: the largest is a relay message, 64 KiB, in
// JSON.
const maxLine = 4 << 20

// JSON-RPC's error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// notification reports whether r expects no answer.
func (r request) notification() bool { return len(r.ID) == 0 }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads requests from in and writes the answers to out until in
// ends or ctx does. Calls run side by side, so a slow one, such as
// waiting on the relay, doesn't hold up the others; the client cancels
// one with notifications/cancelled, and those still running when in
// ends are cancelled.
func (s Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu      sync.Mutex
		enc     = json.NewEncoder(out)
		calls   sync.WaitGroup
		pending = map[string]context.CancelFunc{}
	)
	write := func(r response) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(r)
	}

	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 64<<10), maxLine)
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			write(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, err.Error()}})
			continue
		}
		if req.Method == "notifications/cancelled" {
			var p struct {
				RequestID json.RawMessage `json:"requestId"`
			}
			if json.Unmarshal(req.Params, &p) == nil {
				mu.Lock()
				if stop, ok := pending[string(p.RequestID)]; ok {
					stop()
				}
				mu.Unlock()
			}
			continue
		}
		if req.notification() {
			continue
		}
		callCtx, stop := context.WithCancel(ctx)
		key := string(req.ID)
		mu.Lock()
		pending[key] = stop
		mu.Unlock()
		calls.Go(func() {
			defer func() {
				mu.Lock()
				delete(pending, key)
				mu.Unlock()
				stop()
			}()
			result, rerr := s.answer(callCtx, req)
			if callCtx.Err() != nil && ctx.Err() == nil {
				// Cancelled: the client expects no answer.
				return
			}
			write(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr})
		})
	}
	// The client has gone: so have the calls' answers.
	cancel()
	calls.Wait()
	if err := scan.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// answer is the result of req, or its error.
func (s Server) answer(ctx context.Context, req request) (any, *rpcError) {
	if req.JSONRPC != "2.0" {
		return nil, &rpcError{codeInvalidRequest, "not a JSON-RPC 2.0 request"}
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		result := map[string]any{
			"protocolVersion": Negotiate(p.ProtocolVersion),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}
		if s.Instructions != "" {
			result["instructions"] = s.Instructions
		}
		return result, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := make([]map[string]any, 0, len(s.Tools))
		for _, t := range s.Tools {
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.Schema})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		i := slices.IndexFunc(s.Tools, func(t Tool) bool { return t.Name == p.Name })
		if i < 0 {
			return nil, &rpcError{codeInvalidParams, "no tool " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		text, err := s.Tools[i].Call(ctx, p.Arguments)
		if err != nil {
			return ToolResult(err.Error(), true), nil
		}
		return ToolResult(text, false), nil
	}
	return nil, &rpcError{codeNoMethod, "no method " + req.Method}
}

// ToolResult is a tool call's result: text, which says what went wrong
// when isError.
func ToolResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

// Negotiate is the version the server answers a client asking for
// asked with: the same when the server speaks it, else its newest.
func Negotiate(asked string) string {
	if slices.Contains(Versions, asked) {
		return asked
	}
	return Versions[0]
}

func unmarshal(params json.RawMessage, v any) error {
	if len(params) == 0 {
		return nil
	}
	return json.Unmarshal(params, v)
}
