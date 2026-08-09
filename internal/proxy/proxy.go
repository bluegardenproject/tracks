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
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/dlog"
)

// Entry is one managed proxy: a fixed public port forwarding to an
// optional upstream. All fields except ServiceName, PublicPort and
// BindAll are guarded by mu.
type Entry struct {
	ServiceName string
	PublicPort  int

	// BindAll listens on every interface instead of loopback. Off by
	// default: the proxy fronts a dev server running against the user's
	// own checkout, and binding 0.0.0.0 hands it to anyone on the
	// network — a coffee-shop Wi-Fi away from a stranger's browser.
	// Services that must be reachable from a physical device (a phone
	// loading a Metro bundle) opt in via `proxy_bind_all: true`.
	BindAll bool

	mu       sync.RWMutex
	upstream string                 // "host:port" or "" for inactive
	rp       *httputil.ReverseProxy // cached proxy for the current upstream; nil when inactive
	server   *http.Server           // nil when the listener is not bound
	ln       net.Listener           // nil when the listener is not bound
}

// listenAddr is the address this entry binds when it goes active.
func (e *Entry) listenAddr() string {
	if e.BindAll {
		return fmt.Sprintf(":%d", e.PublicPort)
	}
	return fmt.Sprintf("127.0.0.1:%d", e.PublicPort)
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
	ln, err := net.Listen("tcp", e.listenAddr())
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
			dlog.Printf("proxy %s: %v", name, err)
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

// Registration is one service's stable-port declaration, as it appears
// in the user's config.
type Registration struct {
	ServiceName string
	PublicPort  int
	BindAll     bool
}

// Register declares a proxy entry for the named service. Registration
// does not bind the port — that happens lazily on the first Switch.
// Idempotent: a second call for the same serviceName is silently ignored
// (the first registration wins).
func (m *Manager) Register(r Registration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[r.ServiceName]; !ok {
		m.entries[r.ServiceName] = &Entry{
			ServiceName: r.ServiceName,
			PublicPort:  r.PublicPort,
			BindAll:     r.BindAll,
		}
	}
}

// Sync reconciles the registered entries against the set the config now
// declares: new services are registered, ones whose port or bind changed
// are rebuilt, and ones that disappeared release their port.
//
// Without this the registrations were a startup-only snapshot, so a
// service added through Settings — which the daemon otherwise picks up on
// its next config reload — had no proxy until the daemon was restarted.
//
// An entry that survives unchanged keeps its live listener and upstream,
// so reloading the config never interrupts a track that is serving.
func (m *Manager) Sync(regs []Registration) {
	// replacement pairs a rebuilt entry with the one it supersedes, so the
	// old listener can be released before the new one binds (they may want
	// the same port) and a live upstream can be carried across.
	type replacement struct{ old, new *Entry }

	// The lock is held for the whole body, releases and binds included.
	// Doing that work after the unlock let a concurrent Switch land in
	// between: Switch would bind the new entry and set the upstream for the
	// track the user just started, and the replacement loop below would then
	// overwrite it with the *old* upstream — the stable port quietly serving
	// the previous track's server after a `tracks up` that reported success.
	// Safe because no Entry method reaches back for m.mu, so the order is
	// always m.mu → e.mu and never the reverse; neither net.Listen nor
	// Server.Close blocks on a request handler.
	m.mu.Lock()
	defer m.mu.Unlock()
	var (
		stale        []*Entry
		replacements []replacement
	)
	want := make(map[string]bool, len(regs))
	for _, r := range regs {
		want[r.ServiceName] = true
		cur, ok := m.entries[r.ServiceName]
		if ok && cur.PublicPort == r.PublicPort && cur.BindAll == r.BindAll {
			continue
		}
		next := &Entry{
			ServiceName: r.ServiceName,
			PublicPort:  r.PublicPort,
			BindAll:     r.BindAll,
		}
		m.entries[r.ServiceName] = next
		if ok {
			replacements = append(replacements, replacement{old: cur, new: next})
		}
	}
	for name, e := range m.entries {
		if !want[name] {
			stale = append(stale, e)
			delete(m.entries, name)
		}
	}
	for _, e := range stale {
		e.SetUpstream("")
		e.release()
	}
	// A service whose port or bind changed while it was serving keeps
	// serving. Dropping the upstream here would leave a track that is up
	// and running behind a dead stable port, with nothing said about it —
	// the user's only clue would be a 503 the next time they hit it.
	for _, r := range replacements {
		upstream := r.old.Upstream()
		r.old.SetUpstream("")
		r.old.release()
		if upstream == "" {
			continue
		}
		r.new.SetUpstream(upstream)
		if err := r.new.ensureBound(); err != nil {
			dlog.Printf("proxy %s: config moved it to :%d but that port will not bind (%v) — upstream %s is unreachable through the proxy until the next `tracks up`",
				r.new.ServiceName, r.new.PublicPort, err, upstream)
			r.new.SetUpstream("")
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
// The manager lock is held for the whole call, not just the map lookup.
// Releasing it before ensureBound let a concurrent Sync swap the entry out
// from under us, after which we bound the *old* entry's port — an entry no
// longer in m.entries, so neither Clear nor Stop could ever release it, and
// the replacement never got its upstream. maybeReloadConfig runs at the top
// of every dispatch and each connection is its own goroutine, so two
// clients are enough to hit it. ensureBound only does a net.Listen, so the
// lock is held briefly; Sync never takes an entry lock while holding m.mu,
// so there is no cycle.
func (m *Manager) Switch(serviceName string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[serviceName]
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
