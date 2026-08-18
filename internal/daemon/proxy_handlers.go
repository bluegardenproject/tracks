package daemon

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/dlog"
	"github.com/bluegardenproject/tracks/internal/proxy"
	"github.com/bluegardenproject/tracks/internal/state"
)

// handleProxyAdd defines a new stable port. It is persisted and registered
// with the proxy manager, but not bound until it is linked to an upstream.
func (s *Server) handleProxyAdd(raw json.RawMessage) Response {
	var p ProxyAddParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}
	if p.PublicPort <= 0 {
		return fail("public_port must be a positive port number")
	}

	if err := s.store.PutProxy(state.ProxyBinding{
		PublicPort: p.PublicPort,
		BindAll:    p.BindAll,
	}); err != nil {
		return fail(err.Error())
	}

	if mgr := s.proxyManager(); mgr != nil {
		mgr.Register(proxyRegistration(state.ProxyBinding{PublicPort: p.PublicPort, BindAll: p.BindAll}))
	}
	return ok(nil)
}

// handleProxyRemove deletes a stable port: clears its upstream, releases the
// port, and drops it from state.
func (s *Server) handleProxyRemove(raw json.RawMessage) Response {
	var p ProxyRemoveParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	if mgr := s.proxyManager(); mgr != nil {
		mgr.Remove(p.PublicPort)
	}
	if _, err := s.store.DeleteProxy(p.PublicPort); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

// handleProxySwitch points a stable port at a running track service, or
// clears it. The chosen upstream is persisted on the binding so it survives
// a daemon restart (re-applied while the target is still running).
func (s *Server) handleProxySwitch(raw json.RawMessage) Response {
	var p ProxySwitchParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	mgr := s.proxyManager()
	if mgr == nil {
		return fail("proxy manager not running")
	}

	// The port must be a defined binding.
	if _, ok := s.store.GetProxy(p.PublicPort); !ok {
		return fail(fmt.Sprintf("no stable port :%d defined — add it first", p.PublicPort))
	}

	// TrackID "" or "off" means clear the upstream.
	if p.TrackID == "" || strings.ToLower(p.TrackID) == "off" {
		mgr.Clear(p.PublicPort)
		s.persistProxyUpstream(p.PublicPort, "", "")
		return ok(nil)
	}

	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}
	service, err := resolveTargetService(t, p.Service)
	if err != nil {
		return fail(err.Error())
	}
	port, portFound := t.Ports[service]
	if !portFound {
		return fail(fmt.Sprintf("service %q not in track %s port map", service, p.TrackID))
	}

	if err := mgr.Switch(p.PublicPort, port, p.TrackID, service); err != nil {
		return fail(err.Error())
	}
	s.persistProxyUpstream(p.PublicPort, p.TrackID, service)
	return ok(nil)
}

// handleProxyStatus returns a snapshot of every defined proxy with its
// current upstream and (when active) the track + service it forwards to.
func (s *Server) handleProxyStatus() Response {
	mgr := s.proxyManager()

	var proxies []ProxyEntryStatus
	if mgr != nil {
		for _, ps := range mgr.Status() {
			proxies = append(proxies, ProxyEntryStatus{
				PublicPort:    ps.PublicPort,
				BindAll:       ps.BindAll,
				Upstream:      ps.Upstream,
				ActiveTrackID: ps.UpstreamTrackID,
				ActiveService: ps.UpstreamService,
			})
		}
	}
	return ok(ProxyStatusResult{Proxies: proxies})
}

// releaseTrackProxies frees every stable port forwarding to the given track
// and clears the persisted upstream, so ending or killing a track never
// leaves a port bound to a dead server. Called from the track teardown path.
func (s *Server) releaseTrackProxies(trackID string) {
	mgr := s.proxyManager()
	for _, b := range s.store.AllProxies() {
		if b.UpstreamTrackID != trackID {
			continue
		}
		if mgr != nil {
			mgr.Clear(b.PublicPort)
		}
		s.persistProxyUpstream(b.PublicPort, "", "")
	}
}

// proxyManager returns the live proxy manager under the server lock.
func (s *Server) proxyManager() *proxy.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proxyMgr
}

// proxyRegistration is the manager registration for a persisted binding.
func proxyRegistration(b state.ProxyBinding) proxy.Registration {
	return proxy.Registration{PublicPort: b.PublicPort, BindAll: b.BindAll}
}

// persistProxyUpstream records (or clears) a binding's chosen upstream so it
// survives a restart. The live switch has already happened; a persistence
// failure only means the choice won't survive a restart, so we log it (the
// store does no logging of its own) rather than unwind the switch.
func (s *Server) persistProxyUpstream(port int, trackID, service string) {
	if _, _, err := s.store.UpdateProxy(port, func(b *state.ProxyBinding) bool {
		if b.UpstreamTrackID == trackID && b.UpstreamService == service {
			return false
		}
		b.UpstreamTrackID = trackID
		b.UpstreamService = service
		return true
	}); err != nil {
		dlog.Printf("persist proxy :%d upstream: %v", port, err)
	}
}

// resolveTargetService picks the service on a track a proxy should point at.
// An explicit name must name an active service; an empty name resolves to the
// track's sole active service, and is ambiguous when there are several.
func resolveTargetService(t state.Track, want string) (string, error) {
	var active []string
	for _, sv := range t.Services {
		if sv.Active() {
			active = append(active, sv.Name)
		}
	}
	if want != "" {
		for _, name := range active {
			if name == want {
				return want, nil
			}
		}
		return "", fmt.Errorf("service %q is not running on track %s", want, t.ID)
	}
	switch len(active) {
	case 0:
		return "", fmt.Errorf("track %s has no running services to link", t.ID)
	case 1:
		return active[0], nil
	default:
		return "", fmt.Errorf("track %s runs several services (%s) — specify which one", t.ID, strings.Join(active, ", "))
	}
}
