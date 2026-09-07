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
	EventTrackCreated   Event = "track_created"
	EventWaiting        Event = "waiting"
	EventDone           Event = "done"
	EventErrored        Event = "errored"
	EventPROpened       Event = "pr_opened"
	EventPRStateChanged Event = "pr_state_changed"
	EventServiceReady   Event = "service_ready"
	EventServiceFailed  Event = "service_failed"
)

// AllEvents is the canonical ordering used when defaulting a
// missing config.notify.events list.
var AllEvents = []Event{
	EventTrackCreated,
	EventWaiting,
	EventDone,
	EventErrored,
	EventPROpened,
	EventPRStateChanged,
	EventServiceReady,
	EventServiceFailed,
}

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

// quote wraps s in AppleScript double-quotes, escaping what AppleScript
// treats as special inside one.
//
// Backslashes go first, and that order is the whole point: escaping only
// the quotes left `\"` in a title as `\\"`, which AppleScript reads as a
// literal backslash followed by the string's closing quote. The rest of
// the title then parsed as code — and this string is handed straight to
// `osascript -e`.
//
// The text is not always the user's own typing: bodies and titles are
// built from track slugs (which for a doc review are derived from a
// filename on disk), branch names, PR URLs, service names and log paths.
// git forbids a backslash in a refname but not a double quote, and a
// filename may contain either.
//
// Newlines and tabs get escaped for a duller reason: AppleScript has no
// multi-line string literal, so a raw newline is a syntax error and the
// notification silently never arrives. Control characters it has no
// escape for are dropped — NUL above all, since exec.Command refuses an
// argument containing one, which would lose the notification just as
// quietly.
func quote(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n', r == '\r', r == '\t':
			return r
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return `"` + s + `"`
}
