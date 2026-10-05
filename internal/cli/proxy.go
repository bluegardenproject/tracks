package cli

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// proxySource is the Proxy tab's data, from the daemon.
type proxySource struct{ daemon daemonCalls }

func (p proxySource) call(ctx context.Context, fn func(rpc.Client) (source.ProxyView, error)) (v source.ProxyView, err error) {
	err = p.daemon.do(ctx, func(c rpc.Client) error {
		v, err = fn(c)
		return err
	})
	return v, err
}

func (p proxySource) View(ctx context.Context, fresh bool) (source.ProxyView, error) {
	return p.call(ctx, func(c rpc.Client) (source.ProxyView, error) { return c.Proxy(ctx, fresh) })
}

func (p proxySource) Add(ctx context.Context, port int) (source.ProxyView, error) {
	return p.call(ctx, func(c rpc.Client) (source.ProxyView, error) { return c.AddProxyPort(ctx, port) })
}

func (p proxySource) Remove(ctx context.Context, port int) (source.ProxyView, error) {
	return p.call(ctx, func(c rpc.Client) (source.ProxyView, error) { return c.RemoveProxyPort(ctx, port) })
}

func (p proxySource) SetInput(ctx context.Context, port int, in source.ProxyInput) (source.ProxyView, error) {
	return p.call(ctx, func(c rpc.Client) (source.ProxyView, error) { return c.SetProxyInput(ctx, port, in) })
}
