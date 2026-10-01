package tracksview

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

type liveTracks struct {
	mu     sync.Mutex
	tracks []source.Track
}

func (l *liveTracks) Tracks(context.Context) ([]source.Track, track.Filter, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tracks, track.Filter{}, nil
}

func (l *liveTracks) set(tracks []source.Track) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tracks = tracks
}

// follow runs cmd and what it leads to as Bubble Tea does, each command
// in its own goroutine, until done holds.
func follow(t *testing.T, m Model, cmd tea.Cmd, done func(Model) bool) Model {
	t.Helper()
	msgs := make(chan tea.Msg, 16)
	start := func(c tea.Cmd) {
		if c != nil {
			go func() { msgs <- c() }()
		}
	}
	start(cmd)
	for !done(m) {
		select {
		case msg := <-msgs:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batch {
					start(c)
				}
				continue
			}
			if msg != nil {
				next, c := m.Update(msg)
				m = next.(Model)
				start(c)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("waited too long; the hint row says %q", hintRow(m))
		}
	}
	return m
}

func TestStationFollowsTheChangeStream(t *testing.T) {
	live := &liveTracks{tracks: demoTracks(1)}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	calls := 0
	watch := func(_ context.Context, changed func()) error {
		calls++
		if calls == 1 {
			changed()
			return errors.New("the daemon hung up")
		}
		live.set(demoTracks(2))
		changed()
		<-release
		return nil
	}
	m := New(Config{Version: "test", Theme: theme.Default(), Tracks: live, Watch: watch})
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	down := ""
	m = follow(t, m, tea.Batch(m.watch(), m.nextChange()), func(m Model) bool {
		if m.station.offline && len(m.station.tracks) == 1 {
			down = hintRow(m)
		}
		return down != "" && !m.station.offline && len(m.station.tracks) == 2
	})
	if !strings.Contains(down, "Reconnecting to the daemon…") {
		t.Errorf("hint row while the stream is down = %q", down)
	}
	if row := hintRow(m); strings.Contains(row, "Reconnecting") {
		t.Errorf("hint row once connected again = %q", row)
	}
}
