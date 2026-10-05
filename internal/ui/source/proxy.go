package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

type (
	// ProxyView is the proxy's output ports and the servers they can
	// forward to.
	ProxyView = tracks.ProxyView
	// ProxyStatus is an output port and what it does.
	ProxyStatus = tracks.ProxyStatus
	// Server is a running server a port can forward to.
	Server = tracks.Server
	// ProxyInput is what an output port forwards to.
	ProxyInput = store.ProxyInput
)

// The output ports' states.
const (
	ProxyIdle       = tracks.ProxyIdle
	ProxyForwarding = tracks.ProxyForwarding
	ProxyNoServer   = tracks.ProxyNoServer
	ProxyBlocked    = tracks.ProxyBlocked
)

// InputOf is the input that forwards to sv.
func InputOf(sv Server) ProxyInput { return tracks.InputOf(sv) }

// Proxy reads and changes the proxy's output ports. Fresh reads what
// runs now, with the servers to pick from; else the daemon's last view.
type Proxy interface {
	View(ctx context.Context, fresh bool) (ProxyView, error)
	Add(ctx context.Context, port int) (ProxyView, error)
	Remove(ctx context.Context, port int) (ProxyView, error)
	SetInput(ctx context.Context, port int, in ProxyInput) (ProxyView, error)
}
