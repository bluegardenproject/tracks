package rpc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// serve answers on a short socket path: macOS allows about 100 bytes,
// which a test's temp folder can pass.
func serve(t *testing.T, handlers map[string]Handler) Client {
	dir, err := os.MkdirTemp("/tmp", "rpc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error)
	go func() { done <- Serve(context.Background(), ln, handlers) }()
	t.Cleanup(func() {
		ln.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return Client{Socket: socket}
}

func TestProgressThenResult(t *testing.T) {
	c := serve(t, map[string]Handler{
		Create: func(_ context.Context, call *Call) (any, error) {
			var p CreateParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			call.Progress("Fetching…")
			call.Progress("Starting Claude Code…")
			return CreateResult{ID: "id-1", Name: p.Name, Window: "@3"}, nil
		},
	})
	var steps []string
	got, err := c.Create(context.Background(), CreateParams{Request: tracks.Request{Name: "rate-bug"}}, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if got != (CreateResult{ID: "id-1", Name: "rate-bug", Window: "@3"}) {
		t.Errorf("result = %+v", got)
	}
	if !slices.Equal(steps, []string{"Fetching…", "Starting Claude Code…"}) {
		t.Errorf("progress = %q", steps)
	}
}

func TestErrors(t *testing.T) {
	c := serve(t, map[string]Handler{
		End:  func(context.Context, *Call) (any, error) { return nil, errors.New("tmux is gone") },
		List: func(context.Context, *Call) (any, error) { return nil, tracks.ErrNoEngine },
	})
	ctx := context.Background()
	if err := c.End(ctx, "x"); err == nil || err.Error() != "tmux is gone" {
		t.Errorf("End = %v", err)
	}
	var p tracks.Problem
	if _, err := c.List(ctx); !errors.As(err, &p) || p != tracks.ErrNoEngine {
		t.Errorf("List = %v, want the problem", err)
	}
	if err := c.Shutdown(ctx); err == nil {
		t.Error("an unknown method succeeded")
	}
	if _, err := (Client{Socket: "/tmp/no-such-socket"}).Ping(ctx); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Ping without a daemon = %v", err)
	}
}

func TestHangingUpLeavesTheWorkRunning(t *testing.T) {
	hungUp, finished := make(chan bool, 1), make(chan struct{})
	c := serve(t, map[string]Handler{
		Create: func(ctx context.Context, call *Call) (any, error) {
			call.Progress("Fetching…")
			select {
			case <-call.Gone:
				hungUp <- ctx.Err() == nil
			case <-time.After(5 * time.Second):
				hungUp <- false
			}
			close(finished)
			return CreateResult{}, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	_, err := c.Create(ctx, CreateParams{}, func(string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Create after hanging up = %v", err)
	}
	if !<-hungUp {
		t.Error("the handler didn't see the hang-up, or its context ended with it")
	}
	<-finished
}
