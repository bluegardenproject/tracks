// Package proxy implements the stable-port reverse proxy for dev-server
// services. One proxy listener per user-defined public port; the upstream
// (a running track service, of any name on any track) can be switched
// atomically without restarting the listener.
//
// Listeners are bound lazily: a public port is only claimed while it is
// actively routing to an upstream (from Switch until Clear). An idle
// daemon holds no proxy ports, so a public port that shadows a well-known
// default (e.g. Metro's 8081) stays free for a manual dev server whenever
// no track is using it.
//
// The proxy handles both plain HTTP and WebSocket upgrade requests, so
// HMR (hot-module replacement) works through it without extra wiring.
//
// Ports are user-defined runtime state (persisted in state.json), not a
// per-service config field. The manager is the live view of that state:
// it knows which ports exist and, for each, the upstream it currently
// forwards to. The trackID/service that upstream belongs to are carried
// as labels so status can name the target without a reverse lookup.
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
// optional upstream. All fields except PublicPort and BindAll are guarded
// by mu.
type Entry struct {
	PublicPort int

	// BindAll listens on every interface instead of loopback. Off by
	// default: the proxy fronts a dev server running against the user's
	// own checkout, and binding 0.0.0.0 hands it to anyone on the
	// network — a coffee-shop Wi-Fi away from a stranger's browser.
	// Ports that must be reachable from a physical device (a phone
	// loading a Metro bundle) opt in via BindAll.
	BindAll bool

	mu       sync.RWMutex
	upstream string                 // "host:port" or "" for inactive
	trackID  string                 // owning track of the upstream; "" when inactive
	service  string                 // service name on that track; "" when inactive
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

// UpstreamTarget returns the trackID and service the active upstream
// belongs to, or two empty strings when the entry is inactive.
func (e *Entry) UpstreamTarget() (trackID, service string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.trackID, e.service
}

// setUpstream replaces the active upstream atomically and rebuilds the cached
// reverse proxy. An empty upstream disables forwarding (new requests get 503
// until a new upstream is set); trackID/service are the labels for that
// upstream and are cleared alongside it.
func (e *Entry) setUpstream(upstream, trackID, service string) {
	e.mu.Lock()
	e.upstream = upstream
	e.trackID = trackID
	e.service = service
	if upstream == "" {
		e.rp = nil
		e.trackID = ""
		e.service = ""
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
	go func(srv *http.Server, l net.Listener, port int) {
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			dlog.Printf("proxy :%d: %v", port, err)
		}
	}(e.server, ln, e.PublicPort)
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
		http.Error(w, "no active upstream — link this port to a running dev server", http.StatusServiceUnavailable)
		return
	}

	// Rewrite the Host header so the upstream server accepts the request.
	r.Host = upstream
	rp.ServeHTTP(w, r)
}

// Manager supervises multiple proxy entries, one per user-defined public
// port. It is safe to use from multiple goroutines.
type Manager struct {
	mu      sync.Mutex
	entries map[int]*Entry // public port -> entry
}

// NewManager creates a Manager with no registered entries.
func NewManager() *Manager {
	return &Manager{
		entries: make(map[int]*Entry),
	}
}

// Registration is one public-port declaration, as persisted in state.
type Registration struct {
	PublicPort int
	BindAll    bool
}

// Register declares a proxy entry for a public port. Registration does not
// bind the port — that happens lazily on the first Switch. Idempotent: a
// second call for the same port is silently ignored (the first wins).
func (m *Manager) Register(r Registration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.entries[r.PublicPort]; !ok {
		m.entries[r.PublicPort] = &Entry{
			PublicPort: r.PublicPort,
			BindAll:    r.BindAll,
		}
	}
}

// Remove clears any upstream, releases the port, and drops the entry.
// No-op if the port has no registered proxy.
func (m *Manager) Remove(port int) {
	m.mu.Lock()
	e, ok := m.entries[port]
	if ok {
		delete(m.entries, port)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	e.setUpstream("", "", "")
	e.release()
}

// Sync reconciles the registered entries against the set state now
// declares: new ports are registered, ones whose bind changed are rebuilt,
// and ones that disappeared release their port.
//
// An entry that survives unchanged keeps its live listener and upstream,
// so a reconciliation never interrupts a port that is serving.
func (m *Manager) Sync(regs []Registration) {
	// replacement pairs a rebuilt entry with the one it supersedes, so the
	// old listener can be released before the new one binds (they share a
	// port) and a live upstream can be carried across.
	type replacement struct{ old, new *Entry }

	// The lock is held for the whole body, releases and binds included.
	// Doing that work after the unlock let a concurrent Switch land in
	// between: Switch would bind the new entry and set the upstream for the
	// track the user just started, and the replacement loop below would then
	// overwrite it with the *old* upstream — the stable port quietly serving
	// the previous track's server after a change that reported success.
	// Safe because no Entry method reaches back for m.mu, so the order is
	// always m.mu → e.mu and never the reverse; neither net.Listen nor
	// Server.Close blocks on a request handler.
	m.mu.Lock()
	defer m.mu.Unlock()
	var (
		stale        []*Entry
		replacements []replacement
	)
	want := make(map[int]bool, len(regs))
	for _, r := range regs {
		want[r.PublicPort] = true
		cur, ok := m.entries[r.PublicPort]
		if ok && cur.BindAll == r.BindAll {
			continue
		}
		next := &Entry{
			PublicPort: r.PublicPort,
			BindAll:    r.BindAll,
		}
		m.entries[r.PublicPort] = next
		if ok {
			replacements = append(replacements, replacement{old: cur, new: next})
		}
	}
	for port, e := range m.entries {
		if !want[port] {
			stale = append(stale, e)
			delete(m.entries, port)
		}
	}
	for _, e := range stale {
		e.setUpstream("", "", "")
		e.release()
	}
	// A port whose bind changed while it was serving keeps serving. Dropping
	// the upstream here would leave a track that is up and running behind a
	// dead stable port, with nothing said about it — the user's only clue
	// would be a 503 the next time they hit it.
	for _, r := range replacements {
		upstream := r.old.Upstream()
		trackID, service := r.old.UpstreamTarget()
		r.old.setUpstream("", "", "")
		r.old.release()
		if upstream == "" {
			continue
		}
		r.new.setUpstream(upstream, trackID, service)
		if err := r.new.ensureBound(); err != nil {
			dlog.Printf("proxy :%d: bind after a bind-mode change failed (%v) — upstream %s is unreachable through the proxy until it is re-linked",
				r.new.PublicPort, err, upstream)
			r.new.setUpstream("", "", "")
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

// Switch binds the public port if it isn't already, then points the active
// upstream at "localhost:<upstreamPort>" (the track's allocated service
// port), labelling it with the owning trackID/service. Returns an error if
// the port has no registered proxy or cannot be bound.
//
// The manager lock is held for the whole call, not just the map lookup.
// Releasing it before ensureBound let a concurrent Sync swap the entry out
// from under us, after which we bound the *old* entry's port — an entry no
// longer in m.entries, so neither Clear nor Stop could ever release it, and
// the replacement never got its upstream. ensureBound only does a
// net.Listen, so the lock is held briefly; Sync never takes an entry lock
// while holding m.mu, so there is no cycle.
func (m *Manager) Switch(port, upstreamPort int, trackID, service string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[port]
	if !ok {
		return fmt.Errorf("no proxy registered for port %d", port)
	}
	if err := e.ensureBound(); err != nil {
		return fmt.Errorf("proxy :%d: bind: %w", port, err)
	}
	e.setUpstream(fmt.Sprintf("localhost:%d", upstreamPort), trackID, service)
	return nil
}

// Clear removes the active upstream for a port and releases it so an idle
// daemon holds no proxy ports. The entry stays registered. No-op if the
// port has no registered proxy.
func (m *Manager) Clear(port int) {
	m.mu.Lock()
	e, ok := m.entries[port]
	m.mu.Unlock()
	if !ok {
		return
	}
	e.setUpstream("", "", "")
	e.release()
}

// Status returns a snapshot of every registered proxy entry.
func (m *Manager) Status() []EntryStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]EntryStatus, 0, len(m.entries))
	for _, e := range m.entries {
		trackID, service := e.UpstreamTarget()
		out = append(out, EntryStatus{
			PublicPort:      e.PublicPort,
			BindAll:         e.BindAll,
			Upstream:        e.Upstream(),
			UpstreamTrackID: trackID,
			UpstreamService: service,
		})
	}
	return out
}

// EntryStatus is a point-in-time snapshot of one proxy entry, returned
// by Status and used in the protocol result.
type EntryStatus struct {
	PublicPort int  `json:"public_port"`
	BindAll    bool `json:"bind_all"`
	// Upstream is "host:port" of the active upstream, or "" for inactive.
	Upstream        string `json:"upstream"`
	UpstreamTrackID string `json:"upstream_track_id"`
	UpstreamService string `json:"upstream_service"`
}

// Entry returns the proxy entry for a port, or nil if not registered.
func (m *Manager) Entry(port int) *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[port]
}

// ActivePortFor returns the public port currently forwarding to the given
// track/service, if any. Used to render the stable URL of a running
// service without a reverse lookup on the caller's side.
func (m *Manager) ActivePortFor(trackID, service string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for port, e := range m.entries {
		tID, svc := e.UpstreamTarget()
		if tID == trackID && svc == service && e.Upstream() != "" {
			return port, true
		}
	}
	return 0, false
}
