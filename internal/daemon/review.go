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
	call.Progress("Starting the reviewer. For a Review track, its pull request is read from GitHub first.")
	start, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-call.Gone:
		case <-c.stopping:
		case <-start.Done():
		}
		cancel()
	}()
	name, err := c.Tracks.StartReview(start, p)
	if err != nil {
		c.Log.Printf("starting the reviewer of %s: %v", p.ID, err)
		return nil, err
	}
	call.Progress("The reviewer, " + name + ", runs in a session of its own, which the user opens in an " +
		"overlay (tracks train show reviewer). A review can take minutes. If this command is stopped, the reviewer " +
		"runs on: tracks review --wait waits for its report. If it asks whether to trust the worktree, Tracks tells " +
		"the user, who answers in its overlay.")
	return c.waitReview(ctx, call, p.ID)
}

// reviewWait waits for the report of a track's running review, or its
// answer to the last question.
func (c Config) reviewWait(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ReviewIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	return c.waitReview(ctx, call, p.ID)
}

// reviewOwner is the code owner a track's review narrows to by default.
func (c Config) reviewOwner(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ReviewOwnerParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	owner, err := c.Tracks.ReviewOwner(ctx, p.ID, p.Dir)
	if err != nil {
		return nil, err
	}
	return rpc.ReviewOwnerResult{Owner: owner}, nil
}

// reviewAsk asks a track's reviewer a question and waits for its
// answer.
func (c Config) reviewAsk(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ReviewAskParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.AskReview(ctx, p.ID, p.Question); err != nil {
		return nil, err
	}
	call.Progress("Asked the reviewer. If this command is stopped, tracks review --wait waits for its answer.")
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
