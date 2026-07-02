package daemon

import (
	"encoding/json"
	"fmt"
	"strings"
)

// handleProxySwitch sets or clears the active upstream for a service proxy.
func (s *Server) handleProxySwitch(raw json.RawMessage) Response {
	var p ProxySwitchParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	s.mu.Lock()
	mgr := s.proxyMgr
	s.mu.Unlock()
	if mgr == nil {
		return fail("proxy manager not running (no proxy_port configured in any service?)")
	}

	// TrackID "" or "off" means clear the upstream.
	if p.TrackID == "" || strings.ToLower(p.TrackID) == "off" {
		mgr.Clear(p.ServiceName)
		return ok(nil)
	}

	// Look up the service port for the named service on the given track.
	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}
	port, portFound := t.Ports[p.ServiceName]
	if !portFound {
		return fail(fmt.Sprintf("service %q not in track %s port map", p.ServiceName, p.TrackID))
	}

	if err := mgr.Switch(p.ServiceName, port); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

// handleProxyStatus returns a snapshot of all registered proxies with their
// current upstream and (when determinable) the active track ID.
func (s *Server) handleProxyStatus() Response {
	s.mu.Lock()
	mgr := s.proxyMgr
	s.mu.Unlock()

	var proxies []ProxyEntryStatus
	if mgr != nil {
		statuses := mgr.Status()
		tracks := s.store.All()

		for _, ps := range statuses {
			entry := ProxyEntryStatus{
				ServiceName: ps.ServiceName,
				PublicPort:  ps.PublicPort,
				Upstream:    ps.Upstream,
			}
			// Reverse-lookup: find which track owns this upstream port.
			if ps.Upstream != "" {
				for _, t := range tracks {
					if port, portFound := t.Ports[ps.ServiceName]; portFound {
						if ps.Upstream == fmt.Sprintf("localhost:%d", port) {
							entry.ActiveTrackID = t.ID
							break
						}
					}
				}
			}
			proxies = append(proxies, entry)
		}
	}

	return ok(ProxyStatusResult{Proxies: proxies})
}
