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

// callMethod marshals params and unmarshals result.Result into out.
// out may be nil for methods with no return value.
//
// Implemented on top of callStreaming with a nil progress callback
// so any Progress frames the server emits along the way are
// silently dropped. Without this delegation, methods like Done /
// Kill that grew streaming on the server side would surface the
// first Progress frame to the caller as a Response with Ok=false
// and an empty error message.
func (c *Client) callMethod(method Method, params, out any) error {
	return c.callStreaming(method, params, out, nil)
}

// callStreaming is callMethod's variant that reads zero or more
// Progress frames before the final Response. Progress messages are
// forwarded to onProgress (which may be nil to drop them).
func (c *Client) callStreaming(method Method, params, out any, onProgress func(string)) error {
	socketPath, err := SocketPath(c.cfg)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", socketPath, c.DialTimeout)
	if err != nil {
		return fmt.Errorf("dial daemon socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		paramsRaw = data
	}
	if err := json.NewEncoder(conn).Encode(Request{Method: method, Params: paramsRaw}); err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	// Wire frames either look like {"progress": "..."} (a
	// streaming update) or {"ok": ..., "result": ..., "error": ...}
	// (the final response). We use pointer-typed Ok to distinguish
	// "field missing" from "field is false".
	type wireFrame struct {
		Progress *string         `json:"progress,omitempty"`
		Ok       *bool           `json:"ok,omitempty"`
		Result   json.RawMessage `json:"result,omitempty"`
		Error    string          `json:"error,omitempty"`
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	for {
		var msg wireFrame
		if err := dec.Decode(&msg); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if msg.Progress != nil {
			if onProgress != nil {
				onProgress(*msg.Progress)
			}
			continue
		}
		if msg.Ok == nil {
			return fmt.Errorf("malformed frame: %+v", msg)
		}
		if !*msg.Ok {
			return errors.New(msg.Error)
		}
		if out != nil && len(msg.Result) > 0 {
			if err := json.Unmarshal(msg.Result, out); err != nil {
				return fmt.Errorf("decode result: %w", err)
			}
		}
		return nil
	}
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

// New creates a new track without streaming progress. Prefer
// NewWithProgress for interactive contexts so the user sees the
// fetch / worktree / spawn steps as they happen.
func (c *Client) New(p NewParams) (NewResult, error) {
	var r NewResult
	return r, c.callMethod(MethodNew, p, &r)
}

// NewWithProgress creates a new track and invokes onProgress for
// each progress event the daemon emits before the final response.
// onProgress may be nil, in which case progress events are dropped.
func (c *Client) NewWithProgress(p NewParams, onProgress func(string)) (NewResult, error) {
	var r NewResult
	err := c.callStreaming(MethodNew, p, &r, onProgress)
	return r, err
}

// Done marks a track done and removes its worktrees (keeping
// branches local).
func (c *Client) Done(id string) error {
	return c.callMethod(MethodDone, DoneParams{ID: id}, nil)
}

// DoneWithProgress is Done that forwards Progress frames to
// onProgress while the daemon stops claude and removes
// worktrees. Use when the caller is a popup or other context
// that benefits from a live trace.
func (c *Client) DoneWithProgress(id string, onProgress func(string)) error {
	return c.callStreaming(MethodDone, DoneParams{ID: id}, nil, onProgress)
}

// Kill is Done with prejudice.
func (c *Client) Kill(id string) error {
	return c.callMethod(MethodKill, DoneParams{ID: id}, nil)
}

// KillWithProgress is Kill with progress streaming, see DoneWithProgress.
func (c *Client) KillWithProgress(id string, onProgress func(string)) error {
	return c.callStreaming(MethodKill, DoneParams{ID: id}, nil, onProgress)
}

// AddRepo adds another configured repo to a running track as a new
// worktree on the same branch.
func (c *Client) AddRepo(p AddRepoParams) (AddRepoResult, error) {
	var r AddRepoResult
	return r, c.callMethod(MethodAddRepo, p, &r)
}

// AddRepoWithProgress is AddRepo that streams Progress frames (fetch /
// worktree / provisioning can be slow). Use from interactive callers.
func (c *Client) AddRepoWithProgress(p AddRepoParams, onProgress func(string)) (AddRepoResult, error) {
	var r AddRepoResult
	return r, c.callStreaming(MethodAddRepo, p, &r, onProgress)
}

// PromoteWithProgress turns a worktree-less ask/plan track into a work
// track (creates a worktree + branch and re-spawns Claude), streaming
// progress to the caller.
func (c *Client) PromoteWithProgress(id string, onProgress func(string)) (PromoteResult, error) {
	var r PromoteResult
	return r, c.callStreaming(MethodPromote, PromoteParams{ID: id}, &r, onProgress)
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

// Forget removes a single terminal-state track from persistent
// state. Errors when the track is still running.
func (c *Client) Forget(id string) error {
	return c.callMethod(MethodForget, ForgetParams{ID: id}, nil)
}

// PruneCompleted removes every terminal-state track from
// persistent state. Returns the count removed.
func (c *Client) PruneCompleted() (int, error) {
	var r PruneCompletedResult
	if err := c.callMethod(MethodPruneCompleted, nil, &r); err != nil {
		return 0, err
	}
	return r.Removed, nil
}

// ServiceUpWithProgress starts a named service (and any depends_on deps) for
// a track, streaming progress to onProgress as each service starts and
// becomes ready. Returns the port and log path once the target service is up.
func (c *Client) ServiceUpWithProgress(trackID, serviceName string, onProgress func(string)) (ServiceUpResult, error) {
	var r ServiceUpResult
	err := c.callStreaming(MethodServiceUp, ServiceUpParams{TrackID: trackID, ServiceName: serviceName}, &r, onProgress)
	return r, err
}

// ServiceDown stops a named service for a track, running its pre_stop hooks
// first. Streams progress to onProgress.
func (c *Client) ServiceDownWithProgress(trackID, serviceName string, onProgress func(string)) error {
	return c.callStreaming(MethodServiceDown, ServiceDownParams{TrackID: trackID, ServiceName: serviceName}, nil, onProgress)
}

// Services returns the current service states and allocated port map for
// a track.
func (c *Client) Services(trackID string) (ServicesResult, error) {
	var r ServicesResult
	return r, c.callMethod(MethodServices, ServicesParams{TrackID: trackID}, &r)
}

// ProxySwitch sets the active upstream for a service's stable-port proxy to
// the given track's service port. Pass trackID="" or "off" to clear.
func (c *Client) ProxySwitch(serviceName, trackID string) error {
	return c.callMethod(MethodProxySwitch, ProxySwitchParams{
		ServiceName: serviceName,
		TrackID:     trackID,
	}, nil)
}

// ProxyStatus returns a snapshot of all registered stable-port proxies.
func (c *Client) ProxyStatus() (ProxyStatusResult, error) {
	var r ProxyStatusResult
	return r, c.callMethod(MethodProxyStatus, nil, &r)
}
