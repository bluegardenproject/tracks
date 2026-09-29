package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// ErrNotRunning means no daemon listens on the socket.
var ErrNotRunning = errors.New("the daemon isn't running")

// Client calls the daemon listening on Socket.
type Client struct {
	Socket string
}

// dialTimeout bounds connecting: the daemon is local, so a slow dial is
// a daemon that is stuck.
const dialTimeout = time.Second

// Call sends method with params, gives each progress line to progress
// and decodes the result into result. Either may be nil. Cancelling ctx
// hangs up; the daemon goes on with the work.
func (c Client) Call(ctx context.Context, method string, params, result any, progress func(string)) error {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	defer conn.Close()
	defer context.AfterFunc(ctx, func() { conn.Close() })()

	req := Request{Method: method}
	if params != nil {
		if req.Params, err = json.Marshal(params); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("send %s to the daemon: %w", method, err)
	}
	dec := json.NewDecoder(conn)
	for {
		var r Reply
		if err := dec.Decode(&r); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read the daemon's answer to %s: %w", method, err)
		}
		if !r.Last() {
			if progress != nil {
				progress(r.Progress)
			}
			continue
		}
		switch {
		case r.Error != "" && r.Problem:
			return tracks.Problem(r.Error)
		case r.Error != "":
			return errors.New(r.Error)
		case result != nil && len(r.Result) > 0:
			return json.Unmarshal(r.Result, result)
		}
		return nil
	}
}

func (c Client) Ping(ctx context.Context) (PingResult, error) {
	var r PingResult
	return r, c.Call(ctx, Ping, nil, &r, nil)
}

func (c Client) Shutdown(ctx context.Context) error { return c.Call(ctx, Shutdown, nil, nil, nil) }

func (c Client) Create(ctx context.Context, p CreateParams, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, Create, p, &r, progress)
}

func (c Client) List(ctx context.Context) ([]tracks.Listed, error) {
	var r ListResult
	return r.Tracks, c.Call(ctx, List, nil, &r, nil)
}

func (c Client) End(ctx context.Context, id string) error {
	return c.Call(ctx, End, EndParams{ID: id}, nil, nil)
}

func (c Client) Resume(ctx context.Context, id string, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, Resume, EndParams{ID: id}, &r, progress)
}

// Clean returns the unsaved work it found instead of removing it,
// unless force.
func (c Client) Clean(ctx context.Context, id string, force bool) ([]string, error) {
	var r CleanResult
	return r.Unsaved, c.Call(ctx, Clean, CleanParams{ID: id, Force: force}, &r, nil)
}
