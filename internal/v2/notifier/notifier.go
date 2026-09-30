// Package notifier sends the tracks' notices on the channels the user
// chose: macOS notifications and the terminal bell.
package notifier

import (
	"errors"
	"os"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

// Quiet is how long after telling that a track needs the user it isn't
// told again: an agent asking several questions in a row doesn't flood
// them.
const Quiet = 2 * time.Minute

// Tmux is what the notifier needs from the Tracks tmux server.
type Tmux interface {
	ListWindows(session string) ([]tmux.Window, error)
	ListPanes(window string) ([]tmux.Pane, error)
	ShownWindows(session string) ([]string, error)
}

// Notifier sends notices. Send is safe to call from several goroutines.
type Notifier struct {
	Tmux    Tmux
	Session string // the Tracks session, whose windows are the tracks'
	// Settings reads Settings → General → Notifications, for each notice.
	Settings func() (settings.Settings, error)
	// MacOS shows a macOS notification, Bell rings the terminal of a
	// pane's tty, Now is the clock; nil is the real ones.
	MacOS func(title, body string)
	Bell  func(tty string) error
	Now   func() time.Time
	// Log hears what went wrong; nil drops it.
	Log func(format string, args ...any)

	mu sync.Mutex
	// told is when each track was last told to need the user.
	told map[string]time.Time
}

// Send sends n on each channel that's on, unless its event is off, the
// track's window is on screen, or the track was told it needs the user
// less than Quiet ago.
func (n *Notifier) Send(no tracks.Notice) {
	s, err := n.Settings()
	if err != nil {
		n.log("notify: read settings: %v", err)
		return
	}
	on := s.Notifications
	macOS, bell := on.On(settings.NotifyMacOS), on.On(settings.NotifyBell)
	if !on.On(no.Event) || !macOS && !bell {
		return
	}
	tty, shown, err := n.window(no.Track)
	if err != nil {
		n.log("notify: %s's window: %v", no.Name, err)
	}
	if shown || no.Event == settings.NotifyActionRequired && !n.tell(no.Track) {
		return
	}
	if macOS {
		n.macOS(no.Title, no.Body)
	}
	if bell && tty != "" {
		if err := n.bell(tty); err != nil {
			n.log("notify: ring %s's bell: %v", no.Name, err)
		}
	}
}

// window finds track id's window: the tty of its agent pane, and
// whether a client shows it. A track without a window has neither.
func (n *Notifier) window(id string) (tty string, shown bool, err error) {
	windows, err := n.Tmux.ListWindows(n.Session)
	if err != nil {
		return "", false, err
	}
	i := slices.IndexFunc(windows, func(w tmux.Window) bool { return w.Track == id })
	if i < 0 {
		return "", false, nil
	}
	window := windows[i].ID
	on, err := n.Tmux.ShownWindows(n.Session)
	if err != nil {
		return "", false, err
	}
	if slices.Contains(on, window) {
		return "", true, nil
	}
	panes, err := n.Tmux.ListPanes(window)
	if err != nil {
		return "", false, err
	}
	for _, p := range panes {
		if p.Role == trackwin.RoleAgent {
			return p.TTY, false, nil
		}
	}
	return "", false, nil
}

// tell reports whether track id may be told it needs the user now, and
// notes that it is.
func (n *Notifier) tell(id string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := n.now()
	if last, ok := n.told[id]; ok && now.Sub(last) < Quiet {
		return false
	}
	if n.told == nil {
		n.told = map[string]time.Time{}
	}
	n.told[id] = now
	return true
}

func (n *Notifier) macOS(title, body string) {
	if n.MacOS != nil {
		n.MacOS(title, body)
		return
	}
	notify.New(notify.Channel{MacOS: true}).Send(title, body)
}

func (n *Notifier) bell(tty string) error {
	if n.Bell != nil {
		return n.Bell(tty)
	}
	return ring(tty)
}

// ring writes a BEL to tty. tmux sees it as the pane ringing and
// passes it on to the terminal of its clients.
func ring(tty string) error {
	f, err := os.OpenFile(tty, os.O_WRONLY|syscall.O_NOCTTY, 0)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte{'\a'})
	return errors.Join(err, f.Close())
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n *Notifier) log(format string, args ...any) {
	if n.Log != nil {
		n.Log(format, args...)
	}
}
