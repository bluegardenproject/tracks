package daemon

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
)

// TestMain makes the whole package default-deny on process spawning.
//
// The daemon's spawn points talk to the user's live tmux server, and
// `config.Default()` names the session they actually work in — so a test
// that reaches one of them opens real windows running real `claude` /
// `pnpm install` processes in the user's repos (Bug 7 in
// docs/ROADMAP.md). Installing refusing stubs here rather than in each
// test's helper is the point: protection is a property of the test
// binary, so a new test can't lose it by forgetting a setup line, and
// the failure is a clear message instead of side effects the author may
// not even notice.
//
// Tests that need a spawn to succeed call allowSpawn.
func TestMain(m *testing.M) {
	denySpawns()
	os.Exit(m.Run())
}

// spawnRefusedError is what a test sees when it reaches a spawn point
// without opting in. Phrased as a fix, since the usual cause is a test
// that got further than its author expected.
func spawnRefusedError(what, target string) error {
	return fmt.Errorf("refusing to %s in a test (target %q): this would touch the real tmux server — "+
		"call allowSpawn(t) if the test means to get this far", what, target)
}

// denySpawns points every seam at a refusal. Called by TestMain and by
// allowSpawn's cleanup so state never leaks between tests.
func denySpawns() {
	spawnTrackWindow = func(session, window, _, _ string) (int, error) {
		return 0, spawnRefusedError("open a track window", session+":"+window)
	}
	spawnServicePaneRight = func(session, window, _, _ string, _ int) (string, int, error) {
		return "", 0, spawnRefusedError("split a service pane", session+":"+window)
	}
	spawnServicePaneBelow = func(targetPane, _, _ string) (string, int, error) {
		return "", 0, spawnRefusedError("stack a service pane", targetPane)
	}
}

// The identifiers a recorded spawn hands back must be ones the OS and
// tmux cannot resolve, because the daemon uses them for real afterwards:
// a recorded pane id flows into SetPaneTitle and KillPane, and a recorded
// pid into terminatePGID, which signals -pid and then pid. Plausible
// values (pid 4242, pane "%1") would retitle, kill, or SIGTERM something
// of the user's — the same class of escape as Bug 7.
//
// fakePID is above the kernel's allocatable range, and the pane ids are
// far outside what a real tmux server hands out.
const (
	fakePID        = 1 << 30
	fakePaneFirst  = "%999901"
	fakePaneSecond = "%999902"
)

// spawnCall records one spawn a test allowed.
type spawnCall struct {
	Kind    string // "window" | "pane-right" | "pane-below"
	Target  string // session:window, or the target pane id
	Command string
	CWD     string
}

// spawnRecorder captures allowed spawns instead of performing them.
type spawnRecorder struct {
	mu    sync.Mutex
	calls []spawnCall
}

// Calls returns a copy of what was recorded, in order.
func (r *spawnRecorder) Calls() []spawnCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]spawnCall(nil), r.calls...)
}

func (r *spawnRecorder) add(c spawnCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

// allowSpawn lets the calling test's spawns succeed, recording them
// rather than touching tmux, and restores the package default-deny when
// the test ends. Returned pids/pane ids are synthetic.
//
// Not safe for t.Parallel: the seams are package-level and unsynchronised,
// and they're read from daemon-owned goroutines (handleConn workers,
// supervisor watchers) that a test's cleanup doesn't join — so a swap
// racing a leaked handler is a real data race, not just a logical one.
// No daemon test uses t.Parallel today; -race would catch it if one did.
func allowSpawn(t *testing.T) *spawnRecorder {
	t.Helper()
	rec := &spawnRecorder{}
	spawnTrackWindow = func(session, window, command, cwd string) (int, error) {
		rec.add(spawnCall{Kind: "window", Target: session + ":" + window, Command: command, CWD: cwd})
		return fakePID, nil
	}
	spawnServicePaneRight = func(session, window, command, cwd string, _ int) (string, int, error) {
		rec.add(spawnCall{Kind: "pane-right", Target: session + ":" + window, Command: command, CWD: cwd})
		return fakePaneFirst, fakePID + 1, nil
	}
	spawnServicePaneBelow = func(targetPane, command, cwd string) (string, int, error) {
		rec.add(spawnCall{Kind: "pane-below", Target: targetPane, Command: command, CWD: cwd})
		return fakePaneSecond, fakePID + 2, nil
	}
	t.Cleanup(denySpawns)
	return rec
}

// testConfig returns a config that cannot reach out of the test: state
// under a temp dir, a session name of its own, and notifications off.
//
// The notification part matters as much as the session name. Every
// daemon test used to fail before the notify calls, so nobody had to
// think about it; the moment a test lets a creation succeed, the daemon
// fires a real macOS notification and writes a terminal bell to
// /dev/tty, which tmux renders as an activity marker on whatever window
// the user is looking at.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Tmux.SessionName = "tracks-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	cfg.Notify = config.Notify{}
	return cfg
}
