package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/notifier"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/workspace"
	"github.com/spf13/cobra"
)

// newDaemonCmd runs the daemon. ensureDaemon starts it on the Tracks
// tmux server, so it has the server's environment.
func newDaemonCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:    "daemon",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
			defer stop()
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
				return err
			}
			f, err := os.OpenFile(paths.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			defer f.Close()
			logger := log.New(f, "", log.LstdFlags)
			db, err := store.Open(ctx, paths.Database)
			if err != nil {
				logger.Printf("opening the database: %v", err)
				return err
			}
			defer db.Close()
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			c := tmux.New(paths.TmuxSocket)
			changes := &tracks.Changes{}
			load := func() (settings.Settings, error) { return settings.Load(paths.Settings) }
			notices := &notifier.Notifier{Tmux: c, Session: sessionName, Settings: load, Log: logger.Printf}
			err = daemon.Run(ctx, daemon.Config{
				Paths: paths, Version: version, Session: sessionName, Tmux: c, Home: home, Log: logger,
				Tracks: &tracks.Service{
					Store:     tracks.Watched(db, changes),
					Changes:   changes,
					Worktrees: &workspace.Worktrees{Root: paths.Worktrees},
					Windows:   tracks.TmuxWindows{Tmux: c, Session: sessionName},
					Setups:    tracks.TmuxWindows{Tmux: c, Session: sessionName},
					Servers:   tracks.TmuxWindows{Tmux: c, Session: sessionName},
					Settings:  load,
					SocketDir: paths.DataDir,
					BinDir:    paths.BinDir,
					HooksDir:  filepath.Join(paths.DataDir, "hooks"),
					GitHub:    tracks.GH{},
					// Sending may wait on osascript and tmux; the daemon doesn't.
					Notify: func(n tracks.Notice) { go notices.Send(n) },
				},
			})
			switch {
			case errors.Is(err, daemon.ErrRunning):
				return nil
			case err != nil:
				logger.Printf("exiting: %v", err)
			}
			return err
		},
	}
}

// How long a daemon gets to start, and to finish the creations under
// way when it's asked to shut down.
const (
	daemonStart = 3 * time.Second
	daemonStop  = 10 * time.Second
)

// ensureDaemon returns a client of a daemon running this build, first
// starting one on c's server when there is none or it runs an older
// build.
func ensureDaemon(ctx context.Context, c *tmux.Client, paths platform.Paths, version string) (rpc.Client, error) {
	client := rpc.Client{Socket: paths.Socket}
	if r, err := ping(ctx, client); err == nil {
		if !stale(r, version) {
			return client, nil
		}
		if err := stopDaemon(ctx, client); err != nil {
			return client, err
		}
	}
	command, err := selfCommand()
	if err != nil {
		return client, err
	}
	if err := c.RunShell(command + " daemon >/dev/null 2>&1"); err != nil {
		return client, fmt.Errorf("start the daemon: %w", err)
	}
	for deadline := time.Now().Add(daemonStart); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := ping(ctx, client); err == nil {
			return client, nil
		}
	}
	return client, fmt.Errorf("the daemon didn't start; see %s", paths.Log)
}

// ping bounds a ping: a daemon that doesn't answer in time is stuck.
func ping(ctx context.Context, client rpc.Client) (rpc.PingResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return client.Ping(ctx)
}

// daemonCalls reach the daemon, starting it when a call finds it gone.
type daemonCalls struct {
	tmux    *tmux.Client
	paths   platform.Paths
	version string
}

func (d daemonCalls) do(ctx context.Context, call func(rpc.Client) error) error {
	client := rpc.Client{Socket: d.paths.Socket}
	err := call(client)
	if !errors.Is(err, rpc.ErrNotRunning) {
		return err
	}
	if client, err = ensureDaemon(ctx, d.tmux, d.paths, d.version); err != nil {
		return err
	}
	return call(client)
}

func (d daemonCalls) list(ctx context.Context) (listed []tracks.Listed, err error) {
	err = d.do(ctx, func(c rpc.Client) error {
		listed, err = c.List(ctx)
		return err
	})
	return listed, err
}

// station is Station's list and the filter it's under.
func (d daemonCalls) station(ctx context.Context) (listed []tracks.Listed, f track.Filter, err error) {
	err = d.do(ctx, func(c rpc.Client) error {
		listed, f, err = c.Station(ctx)
		return err
	})
	return listed, f, err
}

// watch follows the daemon's change stream, starting the daemon when it
// isn't running.
func (d daemonCalls) watch(ctx context.Context, changed func()) error {
	return d.do(ctx, func(c rpc.Client) error { return c.Watch(ctx, changed) })
}

// stopDaemon asks the daemon to shut down and waits until it's gone.
func stopDaemon(ctx context.Context, client rpc.Client) error {
	if err := client.Shutdown(ctx); errors.Is(err, rpc.ErrNotRunning) {
		return nil
	} else if err != nil {
		return err
	}
	for deadline := time.Now().Add(daemonStop); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := ping(ctx, client); err != nil {
			return nil
		}
	}
	return errors.New("the daemon is still finishing a track; try again in a moment")
}

// stale reports whether the daemon runs another version, or an older
// build of this binary: a rebuild keeps the version but not the file's
// time. A daemon started from another binary of this version is left
// alone, so two builds don't keep restarting each other's.
func stale(r rpc.PingResult, version string) bool {
	if r.Version != version {
		return true
	}
	self, err := os.Executable()
	if err != nil || r.Exe == "" || !samePath(self, r.Exe) {
		return false
	}
	info, err := os.Stat(self)
	return err == nil && info.ModTime().UnixNano() != r.ExeModified
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
