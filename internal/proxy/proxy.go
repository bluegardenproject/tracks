// Package proxy implements the stable-port reverse proxy for dev-server
// services. One proxy listener per service with a proxy_port configured;
// the upstream (a per-track service port) can be switched atomically
// without restarting the listener.
//
// The proxy handles both plain HTTP and WebSocket upgrade requests, so
// HMR (hot-module replacement) works through it without extra wiring.
package proxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one managed proxy: a fixed public port forwarding to an
// optional upstream. All fields except publicPort are guarded by mu.
type Entry struct {
	ServiceName string
	PublicPort  int

	mu       sync.RWMutex
	upstream string                  // "host:port" or "" for inactive
	rp       *httputil.ReverseProxy  // cached proxy for the current upstream; nil when inactive
	server   *http.Server
	ln       net.Listener
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
	statePath string
}

// persistedState is the JSON shape of proxy.json.
type persistedState struct {
	Upstreams map[string]string `json:"upstreams"` // service name -> "host:port"
}

// NewManager creates a Manager. stateDir is where proxy.json is written.
func NewManager(stateDir string) *Manager {
	return &Manager{
		entries:   make(map[string]*Entry),
		statePath: filepath.Join(stateDir, "proxy.json"),
	}
}

// Register declares a proxy entry for the named service on publicPort.
// Must be called before Start. Idempotent: a second call for the same
// serviceName is silently ignored (the first registration wins).
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

// Start binds each registered proxy port and starts serving in background
// goroutines. It also restores any previously persisted upstream state from
// proxy.json so the proxy survives a daemon restart with its routing intact.
// Returns the first bind error if any listener fails to start.
func (m *Manager) Start() error {
	m.mu.Lock()
	saved := m.loadState()
	entries := make([]*Entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	m.mu.Unlock()

	for _, e := range entries {
		// Restore persisted upstream before binding.
		if up, ok := saved.Upstreams[e.ServiceName]; ok && up != "" {
			e.SetUpstream(up)
		}

		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", e.PublicPort))
		if err != nil {
			return fmt.Errorf("proxy %s: bind :%d: %w", e.ServiceName, e.PublicPort, err)
		}
		e.mu.Lock()
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
		e.mu.Unlock()

		go func(entry *Entry) {
			if err := entry.server.Serve(entry.ln); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "tracks proxy %s: %v\n", entry.ServiceName, err)
			}
		}(e)
	}
	return nil
}

// Stop gracefully shuts down all proxy listeners.
func (m *Manager) Stop() {
	m.mu.Lock()
	entries := make([]*Entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	m.mu.Unlock()

	for _, e := range entries {
		e.mu.Lock()
		srv := e.server
		e.mu.Unlock()
		if srv != nil {
			_ = srv.Close()
		}
	}
}

// Switch sets the active upstream for the named service to "localhost:<port>"
// (using the track's allocated service port), persists the change, and
// returns an error if the service has no registered proxy.
func (m *Manager) Switch(serviceName string, port int) error {
	m.mu.Lock()
	e, ok := m.entries[serviceName]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no proxy registered for service %q", serviceName)
	}
	e.SetUpstream(fmt.Sprintf("localhost:%d", port))
	m.persist()
	return nil
}

// Clear removes the active upstream for the named service (returns 503 until
// another upstream is set). No-op if the service has no registered proxy.
func (m *Manager) Clear(serviceName string) {
	m.mu.Lock()
	e, ok := m.entries[serviceName]
	m.mu.Unlock()
	if !ok {
		return
	}
	e.SetUpstream("")
	m.persist()
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

// persist writes the current upstream state to proxy.json.
// Errors are logged but not returned (best-effort persistence).
func (m *Manager) persist() {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := persistedState{Upstreams: make(map[string]string)}
	for name, e := range m.entries {
		if up := e.Upstream(); up != "" {
			state.Upstreams[name] = up
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "tracks proxy: marshal state: %v\n", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "tracks proxy: mkdir: %v\n", err)
		return
	}
	if err := os.WriteFile(m.statePath, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "tracks proxy: write state: %v\n", err)
	}
}

// loadState reads proxy.json. Returns empty state on any error.
func (m *Manager) loadState() persistedState {
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return persistedState{Upstreams: make(map[string]string)}
	}
	var s persistedState
	if err := json.Unmarshal(data, &s); err != nil {
		return persistedState{Upstreams: make(map[string]string)}
	}
	if s.Upstreams == nil {
		s.Upstreams = make(map[string]string)
	}
	return s
}
