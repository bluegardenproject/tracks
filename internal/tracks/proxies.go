package tracks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// Proxy port states.
const (
	ProxyIdle       = "idle"       // no input: the port isn't listened on
	ProxyForwarding = "forwarding" // to a running server
	ProxyNoServer   = "no-server"  // its input isn't running: it answers 503
	ProxyBlocked    = "blocked"    // another program holds the port, or its input would be itself
)

// ProxyStatus is an output port, its input and what it does.
type ProxyStatus struct {
	Port  int              `json:"port"`
	Input store.ProxyInput `json:"input"`
	// Server is the input as it is now; nil for none, or one that's gone.
	Server *Server `json:"server,omitempty"`
	State  string  `json:"state"`
	// Upstream is the port forwarded to, 0 for none.
	Upstream int `json:"upstream,omitempty"`
	// Holder is the program holding a blocked port, when known.
	Holder string `json:"holder,omitempty"`
}

// ProxyView is the proxy's ports and the servers they can forward to.
type ProxyView struct {
	Ports []ProxyStatus `json:"ports"`
	// Inputs are the running servers of every open track.
	Inputs []Server `json:"inputs"`
}

// ProxyPlan works out each output port's state and the upstream port it
// forwards to, from what runs now; want has the ports to listen on: the
// ones with an input that no other program holds. The caller listens,
// and marks the ports it can't as blocked. Without inputs, which only a
// screen needs, the processes are read only when a port has an input;
// when they can't be read, ProxyPlan fails rather than show every port
// without its server.
func (s *Service) ProxyPlan(ctx context.Context, inputs bool) (view ProxyView, want map[int]int, err error) {
	ports, err := s.Store.ProxyPorts(ctx)
	if err != nil {
		return view, nil, err
	}
	if !inputs && !slices.ContainsFunc(ports, func(p store.ProxyPort) bool { return !p.Input.None() }) {
		for _, p := range ports {
			view.Ports = append(view.Ports, ProxyStatus{Port: p.Port, State: ProxyIdle})
		}
		return view, map[int]int{}, nil
	}
	snap, err := s.takeSnapshot(ctx)
	if err != nil {
		return view, nil, fmt.Errorf("read the processes: %w", err)
	}
	if s.Servers != nil {
		all, err := s.allServers(ctx, snap)
		if err != nil {
			return view, nil, err
		}
		for _, sv := range all {
			if sv.State == ServerReady || sv.State == ServerStarting {
				view.Inputs = append(view.Inputs, sv)
			}
		}
	}
	want = map[int]int{}
	for _, p := range ports {
		st := ProxyStatus{Port: p.Port, Input: p.Input, State: ProxyIdle}
		if p.Input.None() {
			view.Ports = append(view.Ports, st)
			continue
		}
		st.State = ProxyNoServer
		if sv, ok := inputServer(view.Inputs, p.Input); ok {
			st.Server = &sv
			if sv.Port != 0 {
				st.State, st.Upstream = ProxyForwarding, sv.Port
			}
		}
		// A bind on a loopback succeeds on macOS next to a server on
		// every address, so a holder in the snapshot is what tells. The
		// input listening on the port itself would make the proxy
		// forward to itself.
		if h := holder(snap, p.Port); h != "" || st.Upstream == p.Port {
			st.State, st.Upstream, st.Holder = ProxyBlocked, 0, h
		} else {
			want[p.Port] = st.Upstream
		}
		view.Ports = append(view.Ports, st)
	}
	return view, want, nil
}

// inputServer is the server in that in names.
func inputServer(in []Server, input store.ProxyInput) (Server, bool) {
	for _, sv := range in {
		if sv.Track != input.Track {
			continue
		}
		if input.Server != "" && sv.Repo == input.Repo && sv.Name == input.Server {
			return sv, true
		}
		if input.Server == "" && sv.Name == "" && sv.Port == input.Port {
			return sv, true
		}
	}
	return Server{}, false
}

// InputOf is what an output port forwarding to sv stores.
func InputOf(sv Server) store.ProxyInput {
	if sv.Name != "" {
		return store.ProxyInput{Track: sv.Track, Repo: sv.Repo, Server: sv.Name}
	}
	return store.ProxyInput{Track: sv.Track, Port: sv.Port}
}

// holder names the program listening on port, other than this one.
func holder(snap procs.Snapshot, port int) string {
	for pid, ports := range snap.Ports {
		if pid == os.Getpid() {
			continue
		}
		for _, p := range ports {
			if p == port {
				return fmt.Sprintf("%s (pid %d)", procs.Program(snap.Procs[pid].Command), pid)
			}
		}
	}
	return ""
}

// AddProxyPort adds output port, without an input.
func (s *Service) AddProxyPort(ctx context.Context, port int) error {
	switch {
	case port < 1024 || port > 65535:
		return Problem("Enter a port from 1024 to 65535.")
	case port >= track.FirstAssignedPort && port <= track.LastAssignedPort:
		return Problem(fmt.Sprintf("Tracks gives ports %d–%d to dev servers; pick another.", track.FirstAssignedPort, track.LastAssignedPort))
	}
	err := s.Store.AddProxyPort(ctx, port)
	if errors.Is(err, store.ErrPortTaken) {
		return Problem(fmt.Sprintf("Port %d is already listed.", port))
	}
	return err
}

// RemoveProxyPort removes output port.
func (s *Service) RemoveProxyPort(ctx context.Context, port int) error {
	err := s.Store.RemoveProxyPort(ctx, port)
	if errors.Is(err, store.ErrNotFound) {
		return Problem(fmt.Sprintf("Port %d isn't listed.", port))
	}
	return err
}

// SetProxyInput makes output port forward to in, or to nothing for the
// zero input. Switching is only ever this: starting a server never
// moves a port.
func (s *Service) SetProxyInput(ctx context.Context, port int, in store.ProxyInput) error {
	if in.None() {
		in = store.ProxyInput{}
	}
	named := in.Repo != "" && in.Server != "" && in.Port == 0
	found := in.Repo == "" && in.Server == "" && in.Port > 0
	if !in.None() && !named && !found {
		return Problem("An input is a dev server, by repo and name, or a server found on a port.")
	}
	err := s.Store.SetProxyInput(ctx, port, in)
	if errors.Is(err, store.ErrNotFound) {
		return Problem(fmt.Sprintf("Port %d isn't listed.", port))
	}
	return err
}
