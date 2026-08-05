package daemon

import "github.com/bluegardenproject/tracks/internal/tmux"

// The daemon starts long-lived processes in two situations — the tmux
// window a track's Claude runs in, and the pane a dev server runs in
// (which is two calls: the first service splits the window, later ones
// stack under it). All three go through these package-level vars rather
// than calling tmux directly, so tests can replace them.
//
// That indirection is a safety mechanism, not a style choice. tmux calls
// address the *user's live tmux server*: a test that reaches one of
// these with a default config (`Tmux.SessionName` is "tracks") opens
// real windows in whatever session the user is attached to and starts
// real `claude` / `pnpm install` processes in their repos. That is not
// hypothetical — it happened four times while writing the doc-review
// tests, and the pre-existing daemon tests avoid it only by accident,
// failing at an earlier step for unrelated reasons. See Bug 7 in
// docs/ROADMAP.md.
//
// The test binary replaces all three with refusing stubs in TestMain (see
// main_test.go), so the protection is package-wide and a new test cannot
// opt out by forgetting something. Tests that want a spawn to succeed
// call allowSpawn to install a recorder for their duration.
//
// Signatures deliberately mirror the tmux client methods so the
// production wiring stays a one-line delegation.
var (
	// spawnTrackWindow opens the track's window and returns the pid of
	// the pane's process (the group leader).
	spawnTrackWindow = func(session, window, command, cwd string) (int, error) {
		return tmux.New().NewWindowReturningPaneID(session, window, command, cwd)
	}

	// spawnServicePaneRight splits the track window's right column for
	// the first dev server of a track, returning the new pane's id and
	// its process's pid.
	spawnServicePaneRight = func(session, window, command, cwd string, percent int) (string, int, error) {
		return tmux.New().SplitWindowRight(session, window, command, cwd, percent)
	}

	// spawnServicePaneBelow stacks a further dev server under the
	// previous one's pane.
	spawnServicePaneBelow = func(targetPane, command, cwd string) (string, int, error) {
		return tmux.New().SplitPaneDown(targetPane, command, cwd)
	}
)
