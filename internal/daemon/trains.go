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
