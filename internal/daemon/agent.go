package daemon

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// agentCalls are the calls an agent's token opens: those tracks mcp's
// tools make.
var agentCalls = map[string]bool{
	rpc.Agent: true, rpc.RelaySend: true, rpc.RelayRead: true, rpc.RelayShow: true, rpc.RelayWait: true,
	rpc.RelayWho: true, rpc.Servers: true,
}

// agentHandlers are handlers, each answering a call that carries an
// agent's token for the agent's own track, whatever track it names,
// and refusing a call its tools don't make. A call without a token is
// answered as before.
func (c Config) agentHandlers(handlers map[string]rpc.Handler) map[string]rpc.Handler {
	for method, h := range handlers {
		handlers[method] = func(ctx context.Context, call *rpc.Call) (any, error) {
			if call.Token == "" {
				return h(ctx, call)
			}
			if !agentCalls[method] {
				return nil, tracks.Problem(fmt.Sprintf("An agent's token doesn't open %s.", method))
			}
			id, err := c.Tracks.AgentTrack(ctx, call.Token)
			if err != nil {
				return nil, err
			}
			if call.Params, err = withID(call.Params, id); err != nil {
				return nil, err
			}
			return h(ctx, call)
		}
	}
	return handlers
}

// withID is params with their id set to id.
func withID(params json.RawMessage, id string) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &fields); err != nil {
			return nil, err
		}
	}
	fields["id"], _ = json.Marshal(id)
	return json.Marshal(fields)
}

// agent is the track of the agent whose token the call carries.
func (c Config) agent(_ context.Context, call *rpc.Call) (any, error) {
	if call.Token == "" {
		return nil, tracks.Problem("No agent token: tracks mcp runs in an agent Tracks started.")
	}
	var p rpc.AgentResult
	return p, call.Decode(&p)
}
