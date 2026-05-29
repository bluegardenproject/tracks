package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// Client is the CLI-side wrapper for talking to the daemon. Each
// method dials a fresh connection, sends one request, reads one
// response, and closes. This is simpler than connection pooling and
// the request rate is human-scale.
type Client struct {
	cfg config.Config
	// DialTimeout caps how long Dial waits. Short by default so a
	// missing daemon fails fast.
	DialTimeout time.Duration
}

// NewClient constructs a client from config. Dial timeout defaults
// to 2 seconds.
func NewClient(cfg config.Config) *Client {
	return &Client{cfg: cfg, DialTimeout: 2 * time.Second}
}

// roundtrip dials, sends one request, decodes one response.
func (c *Client) roundtrip(req Request) (Response, error) {
	socketPath, err := SocketPath(c.cfg)
	if err != nil {
		return Response{}, err
	}
	conn, err := net.DialTimeout("unix", socketPath, c.DialTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("dial daemon socket %s: %w", socketPath, err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("encode request: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}

// callMethod marshals params and unmarshals result.Result into out.
// out may be nil for methods with no return value.
func (c *Client) callMethod(method Method, params, out any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = data
	}
	resp, err := c.roundtrip(Request{Method: method, Params: paramsRaw})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return errors.New(resp.Error)
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

// Ping returns the daemon's version + PID if reachable.
func (c *Client) Ping() (PingResult, error) {
	var r PingResult
	return r, c.callMethod(MethodPing, nil, &r)
}

// Ls returns every known track.
func (c *Client) Ls() ([]state.Track, error) {
	var r LsResult
	if err := c.callMethod(MethodLs, nil, &r); err != nil {
		return nil, err
	}
	return r.Tracks, nil
}

// Get returns one track by ID. The bool reports whether the track
// was found.
func (c *Client) Get(id string) (state.Track, bool, error) {
	var r GetResult
	if err := c.callMethod(MethodGet, GetParams{ID: id}, &r); err != nil {
		return state.Track{}, false, err
	}
	return r.Track, r.Found, nil
}

// New creates a new track.
func (c *Client) New(p NewParams) (NewResult, error) {
	var r NewResult
	return r, c.callMethod(MethodNew, p, &r)
}

// Done marks a track done and removes its worktrees (keeping
// branches local).
func (c *Client) Done(id string) error {
	return c.callMethod(MethodDone, DoneParams{ID: id}, nil)
}

// Kill is Done with prejudice.
func (c *Client) Kill(id string) error {
	return c.callMethod(MethodKill, DoneParams{ID: id}, nil)
}

// AddRepo adds another configured repo to a running track as a new
// worktree on the same branch.
func (c *Client) AddRepo(p AddRepoParams) (AddRepoResult, error) {
	var r AddRepoResult
	return r, c.callMethod(MethodAddRepo, p, &r)
}

// PendingPrompts returns the daemon's outstanding permission prompts.
func (c *Client) PendingPrompts() ([]PendingPrompt, error) {
	var r PendingPromptsResult
	if err := c.callMethod(MethodPendingPrompts, nil, &r); err != nil {
		return nil, err
	}
	return r.Prompts, nil
}

// AnswerPrompt allows or denies one pending prompt.
func (c *Client) AnswerPrompt(id string, allow bool) error {
	return c.callMethod(MethodAnswerPrompt, AnswerPromptParams{ID: id, Allow: allow}, nil)
}

// Shutdown asks the daemon to exit cleanly.
func (c *Client) Shutdown() error {
	return c.callMethod(MethodShutdown, nil, nil)
}
