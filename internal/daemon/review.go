package daemon

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// review starts a track's reviewer and waits for its report.
func (c Config) review(ctx context.Context, call *rpc.Call) (any, error) {
	var p tracks.ReviewRequest
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	name, err := c.Tracks.StartReview(ctx, p)
	if err != nil {
		c.Log.Printf("starting the reviewer of %s: %v", p.ID, err)
		return nil, err
	}
	call.Progress("The reviewer, " + name + ", runs in a hidden pane of this track, which the footer's button shows. " +
		"A review can take minutes. If this command is stopped, the reviewer runs on: tracks review --wait waits " +
		"for its report. If it waits on a question, such as whether to trust the worktree, the user answers it there.")
	return c.waitReview(ctx, call, p.ID)
}

// reviewWait waits for the report of a track's running review.
func (c Config) reviewWait(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ReviewIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	return c.waitReview(ctx, call, p.ID)
}

// waitReview waits for track id's report until the caller hangs up or
// the daemon stops. The reviewer runs on in its pane either way, and
// tracks review --wait picks its report up later.
func (c Config) waitReview(ctx context.Context, call *rpc.Call, id string) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-call.Gone:
		case <-c.stopping:
		case <-ctx.Done():
		}
		cancel()
	}()
	report, err := c.Tracks.WaitReview(ctx, id)
	if err != nil {
		return nil, err
	}
	return rpc.ReviewResult{Report: report}, nil
}

// reviewReply takes a reviewer's reply from its hook.
func (c Config) reviewReply(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ReviewReplyParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.ReviewReply(ctx, p.ID, p.Run, p.Text); err != nil {
		c.Log.Printf("taking a reply of %s's reviewer: %v", p.ID, err)
		return nil, err
	}
	return nil, nil
}
