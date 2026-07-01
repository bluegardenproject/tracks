package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/state"
)

// Server is the long-running daemon. One process per tmux session.
//
// Lifecycle:
//
//  1. NewServer constructs but does not bind.
//  2. Start opens the socket (and the flock) and launches the accept
//     loop + the tmux-watch loop.
//  3. The tmux-watch loop exits when `tmux has-session` reports the
//     session is gone, at which point Start returns and the process
//     exits.
//  4. Stop is safe to call concurrently with Start.
type Server struct {
	// cfg is swapped atomically by maybeReloadConfig when the on-disk
	// config file changes, so callers always observe a consistent
	// snapshot. Read it via s.config(), never directly.
	cfg      atomic.Pointer[config.Config]
	store    state.Store
	version  string
	notifier *notify.Notifier

	// Config-reload bookkeeping. cfgReloadMu serializes stat+load+swap
	// so concurrent requests don't reload redundantly; the watched
	// mtime/size are the last values we successfully (or unsuccessfully)
	// observed, used to skip reloads when the file is unchanged.
	cfgReloadMu sync.Mutex
	cfgModTime  time.Time
	cfgSize     int64

	// NoTmuxWatch disables the tmux-has-session polling loop. Set to
	// true in tests where there is no tmux session to gate on.
	NoTmuxWatch bool

	socketDir  string
	socketPath string
	lockPath   string

	mu              sync.Mutex
	pendingPrompts  map[string]promptCh
	supervisors     map[string]*supervisor
	listener        net.Listener
	lockFile        *os.File
	stopped         atomic.Bool
	cancelTmuxWatch context.CancelFunc
}

type promptCh struct {
	prompt PendingPrompt
	reply  chan bool
}

// NewServer constructs a Server. The actual sockets are not opened
// until Start. The version string is included in ping responses and
// in the daemon log line.
func NewServer(cfg config.Config, store state.Store, version string) *Server {
	s := &Server{
		store:   store,
		version: version,
		notifier: notify.New(notify.Channel{
			MacOS: cfg.Notify.MacOS,
			Bell:  cfg.Notify.Bell,
		}),
		pendingPrompts: make(map[string]promptCh),
	}
	s.cfg.Store(&cfg)
	return s
}

// config returns the current config snapshot. The returned value is a
// copy of an immutable Config; maybeReloadConfig only ever swaps in a
// fresh pointer, never mutates one in place, so the snapshot stays
// stable for the duration of a caller's use.
func (s *Server) config() config.Config {
	return *s.cfg.Load()
}

// initConfigWatch records the config file's current mtime and size as
// the reload baseline. Called once at startup so that only subsequent
// edits are treated as changes. A missing or unreadable file leaves the
// baseline zeroed, so its later creation registers as a change.
func (s *Server) initConfigWatch() {
	p, err := config.Path()
	if err != nil {
		return
	}
	s.cfgReloadMu.Lock()
	defer s.cfgReloadMu.Unlock()
	if fi, err := os.Stat(p); err == nil {
		s.cfgModTime = fi.ModTime()
		s.cfgSize = fi.Size()
	}
}

// maybeReloadConfig reloads the config file if its mtime or size has
// changed since the last observation, swapping in the new snapshot.
// It is safe to call on every request: when nothing changed it does a
// single stat and returns.
//
// Failure handling is deliberately forgiving — a daemon serving live
// tracks must not break because of a transient stat error or a
// half-saved/malformed edit:
//
//   - A stat error (e.g. the file was momentarily renamed away) leaves
//     the current config untouched and is retried next request.
//   - A parse/validation error logs once and advances the baseline to
//     the bad file's mtime, so we don't re-read and re-log it every
//     request; the user's next edit changes the mtime and we retry.
//
// Infrastructure fields bound at startup (socket/state dirs, tmux
// session name) are preserved from the live config rather than taken
// from disk: the socket and store are already open against the old
// values, so honoring a mid-flight change would diverge the daemon
// from what it actually bound. Those require a real restart.
func (s *Server) maybeReloadConfig() {
	p, err := config.Path()
	if err != nil {
		return
	}
	fi, err := os.Stat(p)
	if err != nil {
		return
	}

	s.cfgReloadMu.Lock()
	defer s.cfgReloadMu.Unlock()

	if fi.ModTime().Equal(s.cfgModTime) && fi.Size() == s.cfgSize {
		return // unchanged
	}

	newCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tracks daemon: config reload failed, keeping previous config: %v\n", err)
		// Advance the baseline so we don't re-read and re-log the same
		// broken file every request; the next edit will retry.
		s.cfgModTime = fi.ModTime()
		s.cfgSize = fi.Size()
		return
	}

	// Preserve startup-bound infrastructure. These cannot change without
	// a restart, so a live edit to them is ignored (not honored).
	cur := s.config()
	newCfg.Paths = cur.Paths
	newCfg.Tmux.SessionName = cur.Tmux.SessionName

	s.cfg.Store(&newCfg)
	s.cfgModTime = fi.ModTime()
	s.cfgSize = fi.Size()
	fmt.Fprintf(os.Stderr, "tracks daemon: reloaded config (%d repos)\n", len(newCfg.Repos))
}

// SocketPath returns the absolute path to the Unix socket. Useful for
// CLI subcommands that need to dial it directly.
func SocketPath(cfg config.Config) (string, error) {
	dir, err := cfg.ResolveSocketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sock"), nil
}

// LockPath returns the absolute path of the startup flock.
func LockPath(cfg config.Config) (string, error) {
	dir, err := cfg.ResolveSocketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sock.lock"), nil
}

// Start blocks until the tmux session is gone or ctx is cancelled.
// Returns nil on a clean tmux-driven shutdown; an error if startup
// failed or another daemon is already running.
func (s *Server) Start(ctx context.Context) error {
	// Record the config file's current mtime/size as the reload
	// baseline, so only edits made *after* startup trigger a reload.
	s.initConfigWatch()

	dir, err := s.config().ResolveSocketDir()
	if err != nil {
		return fmt.Errorf("resolve socket dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir socket dir: %w", err)
	}
	s.socketDir = dir
	s.socketPath = filepath.Join(dir, "sock")
	s.lockPath = filepath.Join(dir, "sock.lock")

	// Acquire the startup flock. If another daemon is already running
	// for this socket dir, this fails fast — we don't want two
	// daemons fighting over one state file.
	if err := s.acquireLock(); err != nil {
		return err
	}
	defer s.releaseLock()

	// Remove a stale socket file from a previous crash. We hold the
	// flock at this point so this is race-free.
	_ = os.Remove(s.socketPath)

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	// Reconcile state before accepting requests so clients don't see
	// a half-stale view of "running" tracks from before the crash.
	s.reconcileOnStartup(ctx)

	// Make sure the global ~/.claude/skills/tracks-add-repo.md and
	// ~/.claude/agents/tracks-reviewer.md are up to date. Non-fatal:
	// an install failure just means the named subagent won't be
	// available and the main agent has to inline its work.
	if err := s.InstallGlobalHelpers(); err != nil {
		fmt.Fprintf(os.Stderr, "tracks daemon: install global helpers: %v\n", err)
	}

	tmuxCtx, cancelTmux := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelTmuxWatch = cancelTmux
	s.mu.Unlock()

	// Tear the daemon down when the root context is cancelled. main's
	// signal.NotifyContext cancels ctx on SIGINT/SIGTERM, but the accept
	// loop blocks in listener.Accept() and never observes ctx on its own,
	// and nothing else closes the listener. Without this, a single stray
	// signal — e.g. one delivered to the process group we inherit from the
	// `tmux run-shell -b` launcher — leaves the daemon wedged: it keeps
	// accepting connections and answering ctx-free requests (ping/ls) while
	// every git-backed one (new/done/promote/…) fails instantly with
	// "context canceled". Stop() closes the listener so acceptLoop returns
	// and Start unblocks; it's idempotent, so racing tmuxWatchLoop's own
	// Stop() is harmless.
	context.AfterFunc(ctx, s.Stop)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.acceptLoop(ctx)
	}()
	if !s.NoTmuxWatch {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.tmuxWatchLoop(tmuxCtx)
		}()
	}

	wg.Wait()
	return nil
}

// Stop closes the listener, signals supervisors to terminate, and
// signals all loops to exit. Safe to call multiple times.
func (s *Server) Stop() {
	if s.stopped.Swap(true) {
		return
	}
	// Read the cancel func under the lock (it's set under the same lock
	// in Start), but invoke it without holding the lock.
	s.mu.Lock()
	cancel := s.cancelTmuxWatch
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Tear down active Claude processes before closing the listener
	// so they get a chance to finalize their logs.
	s.stopAllSupervisors()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

// acceptLoop runs until the listener is closed or ctx is cancelled.
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.stopped.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			fmt.Fprintln(os.Stderr, "tracks daemon: accept:", err)
			return
		}
		go s.handleConn(ctx, conn)
	}
}

// handleConn reads newline-delimited JSON requests from conn,
// dispatches them, and writes responses. One connection per request
// is the simple case; we also support multiple sequential requests
// over the same connection.
//
// Long-running handlers can stream Progress payloads on the same
// connection before the final Response by calling the emit callback
// passed via dispatch. The client decodes either shape and routes
// accordingly.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)
	var encMu sync.Mutex
	emit := func(msg string) {
		encMu.Lock()
		_ = enc.Encode(Progress{Progress: msg})
		encMu.Unlock()
	}
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				encMu.Lock()
				_ = enc.Encode(Response{Ok: false, Error: "decode: " + err.Error()})
				encMu.Unlock()
			}
			return
		}
		resp := s.dispatch(ctx, req, emit)
		encMu.Lock()
		err := enc.Encode(resp)
		encMu.Unlock()
		if err != nil {
			return
		}
		if req.Method == MethodShutdown && resp.Ok {
			// Reply has been written; tear down.
			go s.Stop()
			return
		}
	}
}

// Emit is a progress callback the daemon passes to long-running
// handlers. Short handlers ignore it.
type Emit func(msg string)

// dispatch routes one request to its handler. Handlers live in
// handlers.go so this file stays focused on plumbing.
func (s *Server) dispatch(ctx context.Context, req Request, emit Emit) Response {
	// Pick up edits to the config file (e.g. a newly added repo) before
	// handling the request, so users don't have to restart the daemon.
	s.maybeReloadConfig()

	switch req.Method {
	case MethodPing:
		return s.handlePing()
	case MethodLs:
		return s.handleLs()
	case MethodGet:
		return s.handleGet(req.Params)
	case MethodNew:
		return s.handleNew(ctx, req.Params, emit)
	case MethodDone:
		return s.handleDone(ctx, req.Params, emit)
	case MethodKill:
		return s.handleKill(ctx, req.Params, emit)
	case MethodAddRepo:
		return s.handleAddRepo(ctx, req.Params, emit)
	case MethodPromote:
		return s.handlePromote(ctx, req.Params, emit)
	case MethodPendingPrompts:
		return s.handlePendingPrompts()
	case MethodAnswerPrompt:
		return s.handleAnswerPrompt(req.Params)
	case MethodShutdown:
		return ok(nil)
	case MethodForget:
		return s.handleForget(req.Params)
	case MethodPruneCompleted:
		return s.handlePruneCompleted()
	default:
		return fail(fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// tmuxWatchLoop polls `tmux has-session -t <name>` every 2 seconds
// and triggers Stop() when the session disappears. This is the
// daemon's primary lifecycle gate: kill the tmux session, the
// daemon exits.
//
// We use polling rather than `tmux wait-for` to keep the code path
// simple. 2 seconds is well below human reaction time and 1800
// polls/hour is negligible.
func (s *Server) tmuxWatchLoop(ctx context.Context) {
	name := s.config().Tmux.SessionName
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !tmuxHasSession(name) {
				fmt.Fprintf(os.Stderr, "tracks daemon: tmux session %q gone, exiting\n", name)
				go s.Stop()
				return
			}
		}
	}
}

// tmuxHasSession returns true iff `tmux has-session -t <name>` exits
// zero. Treats tmux-not-installed as "session gone" so the daemon
// doesn't get wedged if tmux is uninstalled while running.
func tmuxHasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}
