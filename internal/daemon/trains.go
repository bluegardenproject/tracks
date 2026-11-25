package daemon

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
)

// trainList lists a track's trains with their status.
func (c Config) trainList(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.TrainListParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	trains, err := c.Tracks.Trains(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := rpc.TrainListResult{Trains: []rpc.Train{}}
	for _, t := range trains {
		out.Trains = append(out.Trains, rpc.Train{Handle: t.Handle, Agent: t.Agent.Name(), Engine: t.Engine,
			Status: t.Status.Label, Outcome: t.Outcome.Label, StatusID: t.Status.ID, OutcomeID: t.Outcome.ID, Held: t.Held})
	}
	return out, nil
}

// trainEnd ends one of a track's trains.
func (c Config) trainEnd(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.TrainEndParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	_, err := c.Tracks.EndTrain(ctx, p.ID, p.Handle)
	return nil, err
}

// signals are what a track's Platform shows under Signals.
func (c Config) signals(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.TrainListParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	sig, err := c.Tracks.Signals(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := rpc.SignalsResult{PRs: []rpc.PRSignal{}, RelayNew: sig.RelayNew, Held: sig.Held}
	for _, pr := range sig.PRs {
		out.PRs = append(out.PRs, rpc.PRSignal{Index: pr.Index, PR: pr.PR, Action: pr.Action, Sent: pr.Sent})
	}
	return out, nil
}
