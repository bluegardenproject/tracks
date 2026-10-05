package daemon

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

func (c Config) up(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ServersParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	servers, err := c.Tracks.Up(ctx, p.ID, p.Server)
	if err != nil {
		c.Log.Printf("starting dev servers of %s: %v", p.ID, err)
		return nil, err
	}
	return rpc.ServersResult{Servers: servers}, nil
}

func (c Config) down(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ServersParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	servers, err := c.Tracks.Down(ctx, p.ID, p.Server)
	if err != nil {
		c.Log.Printf("stopping dev servers of %s: %v", p.ID, err)
		return nil, err
	}
	return rpc.ServersResult{Servers: servers}, nil
}

func (c Config) logs(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.LogsParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	text, err := c.Tracks.Logs(ctx, p.ID, p.Server, p.Lines)
	if err != nil {
		return nil, err
	}
	return rpc.LogsResult{Text: text}, nil
}

func (c Config) servers(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ListServersParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	var servers []tracks.Server
	var err error
	if p.ID == "" {
		servers, err = c.Tracks.AllServers(ctx)
	} else {
		servers, err = c.Tracks.ServerStates(ctx, p.ID)
	}
	if err != nil {
		return nil, err
	}
	return rpc.ServersResult{Servers: servers}, nil
}
