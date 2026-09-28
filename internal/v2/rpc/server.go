package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// maxRequest bounds a request line; a prompt is at most a few KB.
const maxRequest = 1 << 20

// Call is one request being answered.
type Call struct {
	Params json.RawMessage
	// Gone is closed once the caller has hung up.
	Gone <-chan struct{}

	mu  sync.Mutex
	enc *json.Encoder
}

// Progress tells the caller about a slow step, while it listens.
func (c *Call) Progress(msg string) {
	if msg == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.enc.Encode(Reply{Progress: msg})
}

// Decode reads the call's params into v.
func (c *Call) Decode(v any) error {
	if len(c.Params) == 0 {
		return nil
	}
	return json.Unmarshal(c.Params, v)
}

// Handler answers a call. ctx is the server's, not the connection's, so
// the work goes on after the caller hangs up.
type Handler func(ctx context.Context, c *Call) (any, error)

// Serve answers the connections ln accepts with handlers, until ln is
// closed. It returns once the calls being answered are done.
func Serve(ctx context.Context, ln net.Listener, handlers map[string]Handler) error {
	var calls sync.WaitGroup
	defer calls.Wait()
	for {
		conn, err := ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		calls.Go(func() { answer(ctx, conn, handlers) })
	}
}

func answer(ctx context.Context, conn net.Conn, handlers map[string]Handler) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, maxRequest)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Reply{Error: "read the request: " + err.Error()})
		return
	}
	gone := make(chan struct{})
	go func() {
		// The caller sends nothing after its request, so a read ends
		// only when it hangs up.
		_, _ = io.Copy(io.Discard, conn)
		close(gone)
	}()
	call := &Call{Params: req.Params, Gone: gone, enc: json.NewEncoder(conn)}

	reply := Reply{}
	h, ok := handlers[req.Method]
	if !ok {
		reply.Error = fmt.Sprintf("the daemon has no method %q", req.Method)
	} else if result, err := h(ctx, call); err != nil {
		var p tracks.Problem
		reply.Error, reply.Problem = err.Error(), errors.As(err, &p)
	} else if result != nil {
		if reply.Result, err = json.Marshal(result); err != nil {
			reply.Error = err.Error()
		}
	}
	call.mu.Lock()
	defer call.mu.Unlock()
	_ = call.enc.Encode(reply)
}
