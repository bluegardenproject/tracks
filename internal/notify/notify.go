// Package notify pushes attention-grabbing signals out of the
// daemon when a track wants the user back: a macOS notification
// and/or a terminal bell.
//
// The package deliberately has no goroutines, no state, and no
// transport — callers fire-and-forget. Failure to deliver (no tmux,
// no /dev/tty, no osascript) is silent on purpose; we don't want
// the daemon's main loop logging a stream of harmless errors.
package notify

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Event is the kind of state transition that may trigger a
// notification. Strings rather than ints because they round-trip
// cleanly through the YAML allow-list in config.Notify.
type Event string

const (
	EventWaiting        Event = "waiting"
	EventDone           Event = "done"
	EventErrored        Event = "errored"
	EventPROpened       Event = "pr_opened"
	EventPRStateChanged Event = "pr_state_changed"
)

// AllEvents is the canonical ordering used when defaulting a
// missing config.notify.events list.
var AllEvents = []Event{EventWaiting, EventDone, EventErrored, EventPROpened, EventPRStateChanged}

// Channel describes which delivery surfaces are enabled. Independent
// of which events trigger them — events gate "should we notify at
// all?", channels gate "how?".
type Channel struct {
	MacOS bool
	Bell  bool
}

// Notifier sends out one notification on the configured channels.
// Construct once per daemon (cheap; no state).
type Notifier struct {
	Channel Channel
}

// New returns a Notifier with the given channel mix.
func New(ch Channel) *Notifier {
	return &Notifier{Channel: ch}
}

// Send fires a notification with the given title and body on every
// enabled channel. Best-effort: errors are swallowed.
func (n *Notifier) Send(title, body string) {
	if n == nil {
		return
	}
	if n.Channel.MacOS {
		sendMacOS(title, body)
	}
	if n.Channel.Bell {
		sendBell()
	}
}

// sendMacOS shells out to osascript to display a system
// notification. No-op on non-Darwin or when osascript is missing.
func sendMacOS(title, body string) {
	if runtime.GOOS != "darwin" {
		return
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		return
	}
	// AppleScript single-quote escape: ' -> '\''
	script := "display notification " + quote(body) + " with title " + quote(title)
	_ = exec.Command("osascript", "-e", script).Run()
}

// sendBell writes a BEL character to /dev/tty. tmux renders this
// as a status-line activity indicator on any window the user isn't
// currently looking at — exactly the "something needs attention"
// nudge we want.
func sendBell() {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer tty.Close()
	_, _ = tty.Write([]byte("\a"))
}

// quote wraps s in AppleScript double-quotes with internal quotes
// escaped. Used by sendMacOS for both title and body.
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
