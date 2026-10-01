package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
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

// Station is Station's list and the filter it's under.
func (c Client) Station(ctx context.Context) ([]tracks.Listed, track.Filter, error) {
	var r ListResult
	err := c.Call(ctx, List, ListParams{Station: true}, &r, nil)
	return r.Tracks, r.Filter, err
}

// Filter is Station's filter.
func (c Client) Filter(ctx context.Context) (track.Filter, error) {
	var r FilterResult
	return r.Filter, c.Call(ctx, Filter, FilterParams{}, &r, nil)
}

// SetFilter puts Station under f; the zero Filter clears it.
func (c Client) SetFilter(ctx context.Context, f track.Filter) error {
	return c.Call(ctx, Filter, FilterParams{Set: &f}, nil, nil)
}

func (c Client) Unarchive(ctx context.Context, id string) error {
	return c.Call(ctx, Unarchive, UnarchiveParams{ID: id}, nil, nil)
}

// Watch calls changed once connected and after each change to the
// tracks, until ctx ends or the daemon hangs up.
func (c Client) Watch(ctx context.Context, changed func()) error {
	return c.Call(ctx, Watch, nil, nil, func(string) { changed() })
}

func (c Client) End(ctx context.Context, id string) error {
	return c.Call(ctx, End, EndParams{ID: id}, nil, nil)
}

// Report says what happened to a track.
func (c Client) Report(ctx context.Context, p ReportParams) error {
	return c.Call(ctx, Report, p, nil, nil)
}

// Resume returns the worktrees it couldn't find instead of resuming,
// unless p.Recreate.
func (c Client) Resume(ctx context.Context, p ResumeParams, progress func(string)) (ResumeResult, error) {
	var r ResumeResult
	return r, c.Call(ctx, Resume, p, &r, progress)
}

// Promote makes an Ask or Plan track a Work track with its own
// worktrees.
func (c Client) Promote(ctx context.Context, p PromoteParams, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, Promote, p, &r, progress)
}

// Restart starts an open track's agent again, once it exited.
func (c Client) Restart(ctx context.Context, p RestartParams, progress func(string)) (CreateResult, error) {
	var r CreateResult
	return r, c.Call(ctx, Restart, p, &r, progress)
}

// Draft is the request a draft keeps, to start it again.
func (c Client) Draft(ctx context.Context, id string) (tracks.Request, error) {
	var r tracks.Request
	return r, c.Call(ctx, Draft, DraftParams{ID: id}, &r, nil)
}

// DiscardDraft deletes a draft.
func (c Client) DiscardDraft(ctx context.Context, id string) error {
	return c.Call(ctx, DiscardDraft, DraftParams{ID: id}, nil, nil)
}

// Interrupted are the tracks whose windows closed with Tracks, oldest
// first.
func (c Client) Interrupted(ctx context.Context) ([]track.Track, error) {
	var r []track.Track
	return r, c.Call(ctx, Interrupted, nil, &r, nil)
}

// Reopen resumes the interrupted tracks and says how each went.
func (c Client) Reopen(ctx context.Context, progress func(string)) ([]tracks.Reopening, error) {
	var r []tracks.Reopening
	return r, c.Call(ctx, Reopen, nil, &r, progress)
}

// AddRepo gives a work track a worktree of another repo.
func (c Client) AddRepo(ctx context.Context, p AddRepoParams, progress func(string)) (AddRepoResult, error) {
	var r AddRepoResult
	return r, c.Call(ctx, AddRepo, p, &r, progress)
}

// Archive returns the work that would be lost instead of archiving the
// track, when there's some and not p.Force.
func (c Client) Archive(ctx context.Context, p ArchiveParams) ([]string, error) {
	var r LostResult
	return r.Lost, c.Call(ctx, Archive, p, &r, nil)
}

// Derail returns the work that would be lost instead of deleting the
// track, when p.Check or there's some and not p.Force.
func (c Client) Derail(ctx context.Context, p DerailParams) ([]string, error) {
	var r LostResult
	return r.Lost, c.Call(ctx, Derail, p, &r, nil)
}
