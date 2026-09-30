package notifier

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

// fakeTmux has track "a" in window @1, whose agent pane's tty is
// /dev/ttys001, and shows the windows in shown.
type fakeTmux struct{ shown []string }

func (f *fakeTmux) ListWindows(string) ([]tmux.Window, error) {
	return []tmux.Window{{ID: "@0", Index: 0}, {ID: "@1", Index: 1, Track: "a"}}, nil
}

func (f *fakeTmux) ListPanes(window string) ([]tmux.Pane, error) {
	if window != "@1" {
		return nil, fmt.Errorf("no window %s", window)
	}
	return []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, TTY: "/dev/ttys001"}, {ID: "%2", Role: trackwin.RoleTerminal, TTY: "/dev/ttys002"}}, nil
}

func (f *fakeTmux) ShownWindows(string) ([]string, error) { return f.shown, nil }

type rig struct {
	n    *Notifier
	tm   *fakeTmux
	set  settings.Notifications
	now  time.Time
	sent []string
	logs []string
}

func newRig() *rig {
	r := &rig{tm: &fakeTmux{shown: []string{"@0"}}, now: time.UnixMilli(1_790_000_000_000)}
	r.n = &Notifier{
		Tmux:     r.tm,
		Session:  "tracks",
		Settings: func() (settings.Settings, error) { return settings.Settings{Notifications: r.set}, nil },
		MacOS:    func(title, body string) { r.sent = append(r.sent, "macos "+title+": "+body) },
		Bell:     func(tty string) error { r.sent = append(r.sent, "bell "+tty); return nil },
		Now:      func() time.Time { return r.now },
		Log:      func(f string, a ...any) { r.logs = append(r.logs, fmt.Sprintf(f, a...)) },
	}
	return r
}

func (r *rig) send(track, event string) []string {
	r.sent = nil
	r.n.Send(tracks.Notice{Track: track, Name: "rate-bug", Event: event, Title: "T", Body: "B"})
	return r.sent
}

func TestSend(t *testing.T) {
	r := newRig()
	both := []string{"macos T: B", "bell /dev/ttys001"}
	if got := r.send("a", settings.NotifyPROpened); !slices.Equal(got, both) {
		t.Errorf("by default sent %q, want %q: both channels, the bell in the agent pane", got, both)
	}
	if got := r.send("gone", settings.NotifyPRSettled); !slices.Equal(got, both[:1]) {
		t.Errorf("for a track without a window sent %q, want the macOS notification only", got)
	}

	r.set = r.set.Set(settings.NotifyBell, false)
	if got := r.send("a", settings.NotifyError); !slices.Equal(got, both[:1]) {
		t.Errorf("with the bell off sent %q, want the macOS notification only", got)
	}
	r.set = settings.Notifications{}.Set(settings.NotifyMacOS, false)
	if got := r.send("a", settings.NotifyError); !slices.Equal(got, both[1:]) {
		t.Errorf("with macOS off sent %q, want the bell only", got)
	}
	r.set = settings.Notifications{}.Set(settings.NotifyError, false)
	if got := r.send("a", settings.NotifyError); len(got) != 0 {
		t.Errorf("with the event off sent %q, want nothing", got)
	}

	r.set, r.tm.shown = settings.Notifications{}, []string{"@1"}
	if got := r.send("a", settings.NotifyPROpened); len(got) != 0 {
		t.Errorf("for the track on screen sent %q, want nothing", got)
	}
	if len(r.logs) != 0 {
		t.Errorf("logged %q", r.logs)
	}
}

func TestQuiet(t *testing.T) {
	r := newRig()
	if got := r.send("a", settings.NotifyActionRequired); len(got) != 2 {
		t.Fatalf("sent %q, want both channels", got)
	}
	r.now = r.now.Add(Quiet - time.Second)
	if got := r.send("a", settings.NotifyActionRequired); len(got) != 0 {
		t.Errorf("needing the user again within Quiet sent %q, want nothing", got)
	}
	if got := r.send("b", settings.NotifyActionRequired); len(got) != 1 {
		t.Errorf("another track sent %q, want its notice", got)
	}
	if got := r.send("a", settings.NotifyError); len(got) != 2 {
		t.Errorf("another event sent %q, want its notice", got)
	}
	r.now = r.now.Add(time.Second)
	if got := r.send("a", settings.NotifyActionRequired); len(got) != 2 {
		t.Errorf("after Quiet sent %q, want the notice", got)
	}

	r.tm.shown = []string{"@1"}
	r.now = r.now.Add(Quiet)
	r.send("a", settings.NotifyActionRequired)
	r.tm.shown = nil
	if got := r.send("a", settings.NotifyActionRequired); len(got) != 2 {
		t.Errorf("after one skipped on screen sent %q, want the notice: only a sent one starts Quiet", got)
	}
}

func TestFailures(t *testing.T) {
	r := newRig()
	r.n.Settings = func() (settings.Settings, error) { return settings.Settings{}, errors.New("bad yaml") }
	if got := r.send("a", settings.NotifyError); len(got) != 0 || len(r.logs) != 1 {
		t.Errorf("with unreadable settings sent %q and logged %q; want nothing sent, the error logged", got, r.logs)
	}
	r = newRig()
	r.n.Bell = func(string) error { return errors.New("no tty") }
	if got := r.send("a", settings.NotifyError); len(got) != 1 || len(r.logs) != 1 {
		t.Errorf("with a failing bell sent %q and logged %q; want the macOS one sent, the error logged", got, r.logs)
	}
}
