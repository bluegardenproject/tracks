// Package daemon is the v2 daemon: one per user, started on the Tracks
// tmux server, it creates and ends tracks for the CLI and the popups
// and notices when a track's window closes.
package daemon

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// ErrRunning means another daemon holds the lock.
var ErrRunning = errors.New("another daemon is running")

// every is how often the windows are checked, pollEvery how often
// GitHub is asked about the tracks' PRs, archiveEvery how often old
// tracks are archived.
const (
	every        = 2 * time.Second
	pollEvery    = time.Minute
	archiveEvery = time.Hour
)

// Config is what the daemon runs with.
type Config struct {
	Paths   platform.Paths
	Version string
	Tracks  *tracks.Service
	// Tmux is the Tracks server: the daemon exits once Session is gone,
	// and tells a client how a creation went when its form closed.
	Tmux interface {
		HasSession(name string) bool
		Tell(client, msg string) error
	}
	Session string
	// Home is where the reviewer subagents are installed.
	Home string
	Log  *log.Logger
	// Every is how often the windows are checked; 0 is 2 s. PollEvery is
	// how often the PRs are; 0 is a minute.
	Every, PollEvery time.Duration
}

// Run serves until ctx ends, a client asks it to shut down or the tmux
// session is gone. Creations under way are finished first.
func Run(ctx context.Context, c Config) error {
	if err := os.MkdirAll(c.Paths.DataDir, 0o700); err != nil {
		return err
	}
	unlock, err := lock(c.Paths.Lock)
	if err != nil {
		return err
	}
	defer unlock()
	// The lock is ours, so a socket that is there was left behind.
	if err := os.Remove(c.Paths.Socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", c.Paths.Socket)
	if err != nil {
		return err
	}
	defer os.Remove(c.Paths.Socket)
	if err := os.Chmod(c.Paths.Socket, 0o600); err != nil {
		ln.Close()
		return err
	}
	c.helpers()

	stop := make(chan struct{})
	var once sync.Once
	shutdown := func() { once.Do(func() { close(stop) }) }
	served := make(chan error, 1)
	go func() { served <- rpc.Serve(context.WithoutCancel(ctx), ln, c.handlers(shutdown)) }()

	interval := c.Every
	if interval == 0 {
		interval = every
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	pollInterval := c.PollEvery
	if pollInterval == 0 {
		pollInterval = pollEvery
	}
	polls := time.NewTicker(pollInterval)
	defer polls.Stop()
	pollCtx, cancelPoll := context.WithCancel(ctx)
	prs := &poller{log: c.Log, what: "checking the pull requests"}
	defer prs.wait()
	archives := time.NewTicker(archiveEvery)
	defer archives.Stop()
	archiver := &poller{log: c.Log, what: "archiving old tracks"}
	defer archiver.wait()
	defer cancelPoll()

	c.Log.Printf("started, pid %d", os.Getpid())
	prs.start(pollCtx, c.Tracks.PollPRs)
	archiver.start(pollCtx, c.autoArchive)
	reason := ""
	for reason == "" {
		select {
		case <-ctx.Done():
			reason = "stopped"
		case <-stop:
			reason = "asked to shut down"
		case err := <-served:
			return err
		case <-polls.C:
			prs.start(pollCtx, c.Tracks.PollPRs)
		case <-archives.C:
			archiver.start(pollCtx, c.autoArchive)
		case <-tick.C:
			if !c.Tmux.HasSession(c.Session) {
				reason = "the tmux session is gone"
			} else if err := c.Tracks.Sweep(ctx); err != nil {
				c.Log.Printf("checking the windows: %v", err)
			} else if err := c.Tracks.CheckScreens(ctx); err != nil {
				c.Log.Printf("checking the agents' screens: %v", err)
			}
		}
	}
	c.Log.Printf("exiting: %s", reason)
	c.Tracks.Changes.Close()
	ln.Close()
	return <-served
}

// lock takes the lock file at path, or fails with ErrRunning while
// another process holds it. Closing the file releases it, so a daemon
// that dies leaves no stale lock.
func lock(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunning
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}
