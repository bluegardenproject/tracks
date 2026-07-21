// Package proxy implements the stable-port reverse proxy for dev-server
// services. One proxy listener per service with a proxy_port configured;
// the upstream (a per-track service port) can be switched atomically
// without restarting the listener.
//
// Listeners are bound lazily: a proxy port is only claimed while a track
// is actively routing through it (from Switch until Clear). An idle
// daemon holds no proxy ports, so a proxy_port that shadows a well-known
// default (e.g. Metro's 8081) stays free for a manual dev server whenever
// no track is using it.
//
// The proxy handles both plain HTTP and WebSocket upgrade requests, so
// HMR (hot-module replacement) works through it without extra wiring.
package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"
)

// Entry is one managed proxy: a fixed public port forwarding to an
// optional upstream. All fields except ServiceName and PublicPort are
// guarded by mu.
type Entry struct {
	ServiceName string
	PublicPort  int

	mu       sync.RWMutex
	upstream string                 // "host:port" or "" for inactive
	rp       *httputil.ReverseProxy // cached proxy for the current upstream; nil when inactive
	server   *http.Server           // nil when the listener is not bound
	ln       net.Listener           // nil when the listener is not bound
}

// Upstream returns the current upstream ("host:port"), or "" if inactive.
func (e *Entry) Upstream() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.upstream
}

// SetUpstream replaces the active upstream atomically and rebuilds the cached
// reverse proxy. An empty string disables forwarding (new requests get 503
// until a new upstream is set).
func (e *Entry) SetUpstream(upstream string) {
	e.mu.Lock()
	e.upstream = upstream
	if upstream == "" {
		e.rp = nil
	} else {
		target := &url.URL{Scheme: "http", Host: upstream}
		rp := httputil.NewSingleHostReverseProxy(target)
		// FlushInterval -1 disables response buffering; required for
		// Server-Sent Events and streaming responses like Metro's bundle
		// delivery.
		rp.FlushInterval = -1
		rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		}
		e.rp = rp
	}
	e.mu.Unlock()
}

// ensureBound binds the public port and starts serving in a background
// goroutine, unless the listener is already bound. Safe to call repeatedly.
func (e *Entry) ensureBound() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", e.PublicPort))
	if err != nil {
		return err
	}
	e.ln = ln
	e.server = &http.Server{
		Handler:      e,
		ReadTimeout:  0, // no timeout — streaming responses like Metro bundles can take long
		WriteTimeout: 0,
		// IdleTimeout closes truly idle keep-alive connections; 60 s is
		// conservative enough that HMR WebSockets stay open between saves
		// while still releasing abandoned connections.
		IdleTimeout: 60 * time.Second,
	}
	go func(srv *http.Server, l net.Listener, name string) {
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "tracks proxy %s: %v\n", name, err)
		}
	}(e.server, ln, e.ServiceName)
	return nil
}

// release closes the listener and server if bound, freeing the public port.
// No-op when not bound. The listener is closed directly (not just via
// srv.Close) so the port is freed synchronously: srv.Close races the
// Serve goroutine registering its listener and may otherwise miss it.
func (e *Entry) release() {
	e.mu.Lock()
	srv := e.server
	ln := e.ln
	e.server = nil
	e.ln = nil
	e.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
	if ln != nil {
		_ = ln.Close()
	}
}

func (e *Entry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	upstream := e.upstream
	rp := e.rp
	e.mu.RUnlock()

	if rp == nil || upstream == "" {
		http.Error(w, "no active upstream — run `tracks proxy switch` to activate one", http.StatusServiceUnavailable)
		return
	}

	// Rewrite the Host header so the upstream server accepts the request.
	r.Host = upstream
	rp.ServeHTTP(w, r)
}

// Manager supervises multiple proxy entries (one per service with a proxy_port).
// It is safe to use from multiple goroutines.
type Manager struct {
	mu      sync.Mutex
	entries map[string]*Entry // service name -> entry
}

// NewManager creates a Manager with no registered entries.
func NewManager() *Manager {
	return &Manager{
		entries: make(map[string]*Entry),
	}
}

// Register declares a proxy entry for the named service on publicPort.
// Registration does not bind the port — that happens lazily on the first
// Switch. Idempotent: a second call for the same serviceName is silently
// ignored (the first registration wins).
func (m *Manager) Register(serviceName string, publicPort int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[serviceName]; !ok {
		m.entries[serviceName] = &Entry{
			ServiceName: serviceName,
			PublicPort:  publicPort,
		}
	}
}

// Stop gracefully shuts down all bound proxy listeners.
func (m *Manager) Stop() {
	m.mu.Lock()
	entries := make([]*Entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	m.mu.Unlock()

	for _, e := range entries {
		e.release()
	}
}

// Switch binds the service's public port if it isn't already, then points
// the active upstream at "localhost:<port>" (the track's allocated service
// port). Returns an error if the service has no registered proxy or the
// port cannot be bound.
func (m *Manager) Switch(serviceName string, port int) error {
	m.mu.Lock()
	e, ok := m.entries[serviceName]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no proxy registered for service %q", serviceName)
	}
	if err := e.ensureBound(); err != nil {
		return fmt.Errorf("proxy %s: bind :%d: %w", serviceName, e.PublicPort, err)
	}
	e.SetUpstream(fmt.Sprintf("localhost:%d", port))
	return nil
}

// Clear removes the active upstream for the named service and releases its
// public port so an idle daemon holds no proxy ports. No-op if the service
// has no registered proxy.
func (m *Manager) Clear(serviceName string) {
	m.mu.Lock()
	e, ok := m.entries[serviceName]
	m.mu.Unlock()
	if !ok {
		return
	}
	e.SetUpstream("")
	e.release()
}

// Status returns a snapshot of every registered proxy entry.
func (m *Manager) Status() []EntryStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]EntryStatus, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, EntryStatus{
			ServiceName: e.ServiceName,
			PublicPort:  e.PublicPort,
			Upstream:    e.Upstream(),
		})
	}
	return out
}

// EntryStatus is a point-in-time snapshot of one proxy entry, returned
// by Status and used in the protocol result.
type EntryStatus struct {
	ServiceName string `json:"service_name"`
	PublicPort  int    `json:"public_port"`
	// Upstream is "host:port" of the active upstream, or "" for inactive.
	Upstream string `json:"upstream"`
}

// Entry returns the proxy entry for a service, or nil if not registered.
// Callers use this to check if a service has a configured proxy_port.
func (m *Manager) Entry(serviceName string) *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[serviceName]
}
