package daemon

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/rpc"
)

// The relay's calls come from a track's agent, which reads as the
// track, and from the Relay Station, which reads for the user.

func (c Config) relaySend(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelaySendParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	m, err := c.Tracks.RelaySend(ctx, p.ID, p.To, p.Kind, p.Body)
	if err != nil {
		return nil, err
	}
	return rpc.RelayMessages{Messages: []relay.Message{m}}, nil
}

func (c Config) relayRead(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	msgs, err := c.Tracks.RelayRead(ctx, p.ID, relay.TrackHandle)
	return rpc.RelayMessages{Messages: msgs}, err
}

func (c Config) relayShow(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayShowParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	m, err := c.Tracks.RelayShow(ctx, p.ID, relay.TrackHandle, p.Msg)
	if err != nil {
		return nil, err
	}
	return rpc.RelayMessages{Messages: []relay.Message{m}}, nil
}

// relayWait waits until the caller hangs up or the daemon stops.
func (c Config) relayWait(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
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
	msgs, err := c.Tracks.RelayWait(ctx, p.ID, relay.TrackHandle)
	return rpc.RelayMessages{Messages: msgs}, err
}

func (c Config) relayWho(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	members, err := c.Tracks.RelayWho(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := rpc.RelayWhoResult{Members: []rpc.Member{}}
	for _, m := range members {
		out.Members = append(out.Members, rpc.Member{Handle: m.Handle, Agent: m.Agent, Engine: m.Engine,
			Status: m.Status.Label, Outcome: m.Outcome.Label})
	}
	return out, nil
}

func (c Config) relayStation(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	log, err := c.Tracks.RelayStation(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return rpc.RelayStationResult{Messages: log.Messages, Waiting: log.Waiting}, nil
}

func (c Config) relayNew(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RelayIDParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	n, err := c.Tracks.RelayNew(ctx, p.ID)
	return rpc.RelayNewResult{New: n}, err
}
