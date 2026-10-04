package daemon

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

func (c Config) setup(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.SetupParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	var states []tracks.SetupRepo
	var err error
	if p.Wait {
		// Waiting ends with the caller: ctx is the server's.
		wait, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-call.Gone:
				cancel()
			case <-wait.Done():
			}
		}()
		states, err = c.Tracks.WaitSetup(wait, p.ID)
	} else {
		states, err = c.Tracks.StartSetup(ctx, p.ID, p.Retry)
	}
	if err != nil {
		c.Log.Printf("setup of %s: %v", p.ID, err)
		return nil, err
	}
	return rpc.SetupResult{Repos: states}, nil
}

func (c Config) setupDone(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.SetupDoneParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.FinishSetup(ctx, p.ID, p.Repo); err != nil {
		return nil, err
	}
	c.Log.Printf("setup of %s in %s done", p.Repo, p.ID)
	return nil, nil
}
