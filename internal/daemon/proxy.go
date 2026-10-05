package daemon

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bluegardenproject/tracks/internal/proxy"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// proxies is the daemon's proxy, kept in step with the output ports and
// what runs behind them.
type proxies struct {
	mu sync.Mutex
	p  proxy.Proxy
}

// syncProxy points the proxy at what each output port's input runs on
// now, and returns the ports' states; inputs adds the servers they can
// forward to. When the processes can't be read the proxy stays as it
// was. RPCs wait for a sync under way, which reads the processes.
func (c Config) syncProxy(ctx context.Context, inputs bool) (tracks.ProxyView, error) {
	if c.proxies == nil {
		return tracks.ProxyView{}, errors.New("the proxy isn't running")
	}
	c.proxies.mu.Lock()
	defer c.proxies.mu.Unlock()
	view, want, err := c.Tracks.ProxyPlan(ctx, inputs)
	if err != nil {
		return view, err
	}
	blocked := c.proxies.p.Set(want)
	for i, st := range view.Ports {
		if blocked[st.Port] != nil {
			view.Ports[i].State = tracks.ProxyBlocked
		}
	}
	return view, nil
}

func (c Config) proxyView(ctx context.Context, _ *rpc.Call) (any, error) {
	return c.syncProxy(ctx, true)
}

func (c Config) proxyAdd(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ProxyPortParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.AddProxyPort(ctx, p.Port); err != nil {
		return nil, err
	}
	return c.syncAfterChange(ctx)
}

func (c Config) proxyRemove(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ProxyPortParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.RemoveProxyPort(ctx, p.Port); err != nil {
		return nil, err
	}
	return c.syncAfterChange(ctx)
}

func (c Config) proxyInput(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ProxyPortParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	if err := c.Tracks.SetProxyInput(ctx, p.Port, p.Input); err != nil {
		return nil, err
	}
	c.Log.Printf("proxy port %d forwards to %+v", p.Port, p.Input)
	return c.syncAfterChange(ctx)
}

// syncAfterChange syncs the proxy after a saved change. A sync that
// fails says the change was saved, so it isn't taken for a failed one;
// the next tick catches the proxy up.
func (c Config) syncAfterChange(ctx context.Context) (any, error) {
	view, err := c.syncProxy(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("saved, but the proxy isn't up to date yet: %w", err)
	}
	return view, nil
}
