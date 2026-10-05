package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

func (c Config) handlers(shutdown func()) map[string]rpc.Handler {
	ping := rpc.PingResult{Version: c.Version, PID: os.Getpid()}
	if exe, err := os.Executable(); err == nil {
		ping.Exe = exe
		if info, err := os.Stat(exe); err == nil {
			ping.ExeModified = info.ModTime().UnixNano()
		}
	}
	return map[string]rpc.Handler{
		rpc.Ping: func(context.Context, *rpc.Call) (any, error) { return ping, nil },
		rpc.Shutdown: func(context.Context, *rpc.Call) (any, error) {
			shutdown()
			return nil, nil
		},
		rpc.Create: c.create,
		rpc.Watch: func(_ context.Context, call *rpc.Call) (any, error) {
			changed, cancel := c.Tracks.Changes.Subscribe()
			defer cancel()
			for {
				select {
				case _, ok := <-changed:
					if !ok {
						return nil, nil
					}
					call.Progress("changed")
				case <-call.Gone:
					return nil, nil
				}
			}
		},
		rpc.List: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.ListParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			if !p.Station {
				listed, err := c.Tracks.List(ctx)
				return rpc.ListResult{Tracks: listed}, err
			}
			listed, f, err := c.Tracks.Station(ctx)
			return rpc.ListResult{Tracks: listed, Filter: f}, err
		},
		rpc.Filter: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.FilterParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			if p.Set != nil {
				if err := c.Tracks.SetFilter(ctx, *p.Set); err != nil {
					return nil, err
				}
			}
			f, err := c.Tracks.Filter(ctx)
			return rpc.FilterResult{Filter: f}, err
		},
		rpc.Unarchive: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.UnarchiveParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			if err := c.Tracks.Unarchive(ctx, p.ID); err != nil {
				return nil, err
			}
			c.Log.Printf("unarchived %s", p.ID)
			return nil, nil
		},
		rpc.End: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.EndParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			return nil, c.Tracks.End(ctx, p.ID)
		},
		rpc.Resume:  c.resume,
		rpc.Archive: c.archive,
		rpc.Derail:  c.derail,
		rpc.AddRepo: c.addRepo,
		rpc.Promote: c.promote,
		rpc.Restart: c.restart,
		rpc.Draft: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.DraftParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			return c.Tracks.Draft(ctx, p.ID)
		},
		rpc.DiscardDraft: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.DraftParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			return nil, c.Tracks.DiscardDraft(ctx, p.ID)
		},
		rpc.Interrupted: func(ctx context.Context, _ *rpc.Call) (any, error) {
			return c.Tracks.Interrupted(ctx)
		},
		rpc.Reopen:    c.reopen,
		rpc.Setup:     c.setup,
		rpc.SetupDone: c.setupDone,
		rpc.Up:        c.up,
		rpc.Down:      c.down,
		rpc.Logs:      c.logs,
		rpc.Report: func(ctx context.Context, call *rpc.Call) (any, error) {
			var p rpc.ReportParams
			if err := call.Decode(&p); err != nil {
				return nil, err
			}
			if p.Event != "" {
				if err := c.Tracks.Report(ctx, p.ID, track.Event(p.Event)); err != nil {
					return nil, err
				}
			}
			return nil, c.Tracks.SeePRs(ctx, p.ID, p.PRs)
		},
	}
}

func (c Config) resume(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ResumeParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	resumed, err := c.Tracks.Resume(ctx, p.ID, p.Recreate, call.Progress)
	var missing tracks.Missing
	if errors.As(err, &missing) {
		c.Log.Printf("resuming %s: %v", p.ID, err)
		var r rpc.ResumeResult
		for _, repo := range missing {
			r.Missing = append(r.Missing, repo.Name+": "+repo.Worktree)
		}
		return r, nil
	}
	if err != nil {
		c.Log.Printf("resuming %s failed: %v", p.ID, err)
		return nil, err
	}
	c.Log.Printf("resumed %s, %s", p.ID, resumed.Track.Name)
	return rpc.ResumeResult{CreateResult: rpc.CreateResult{ID: p.ID, Name: resumed.Track.Name, Window: resumed.Window.ID}}, nil
}

func (c Config) reopen(ctx context.Context, call *rpc.Call) (any, error) {
	reopened, err := c.Tracks.Reopen(ctx, call.Progress)
	for _, r := range reopened {
		if r.Error != "" {
			c.Log.Printf("reopening %s failed: %s", r.ID, r.Error)
		} else {
			c.Log.Printf("reopened %s, %s", r.ID, r.Name)
		}
	}
	return reopened, err
}

func (c Config) promote(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.PromoteParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	promoted, err := c.Tracks.Promote(ctx, p.ID, call.Progress)
	if err != nil {
		c.Log.Printf("promoting %s failed: %v", p.ID, err)
		return nil, err
	}
	c.Log.Printf("promoted %s, %s, to a Work track", p.ID, promoted.Track.Name)
	return rpc.CreateResult{ID: p.ID, Name: promoted.Track.Name, Window: promoted.Window.ID}, nil
}

func (c Config) restart(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.RestartParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	restarted, err := c.Tracks.Restart(ctx, p.ID, call.Progress)
	if err != nil {
		c.Log.Printf("restarting %s failed: %v", p.ID, err)
		return nil, err
	}
	c.Log.Printf("restarted %s's agent", restarted.Track.Name)
	return rpc.CreateResult{ID: p.ID, Name: restarted.Track.Name, Window: restarted.Window.ID}, nil
}

func (c Config) addRepo(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.AddRepoParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	r, err := c.Tracks.AddRepo(ctx, p.ID, p.Repo, call.Progress)
	if err != nil {
		c.Log.Printf("adding %s to %s failed: %v", p.Repo, p.ID, err)
		return nil, err
	}
	c.Log.Printf("added %s to %s at %s", r.Name, p.ID, r.Worktree)
	return rpc.AddRepoResult{Name: r.Name, Worktree: r.Worktree}, nil
}

func (c Config) archive(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ArchiveParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	lost, err := c.Tracks.Archive(ctx, p.ID, p.Force)
	if err != nil {
		c.Log.Printf("archiving %s failed: %v", p.ID, err)
		return nil, err
	}
	r := lostResult(lost)
	if len(r.Lost) == 0 {
		c.Log.Printf("archived %s", p.ID)
	}
	return r, nil
}

func (c Config) derail(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.DerailParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	var lost []workspace.Unsaved
	var err error
	if p.Check {
		lost, err = c.Tracks.Lost(ctx, p.ID)
	} else if lost, err = c.Tracks.Derail(ctx, p.ID, p.Force); err == nil && len(lost) == 0 {
		c.Log.Printf("derailed %s", p.ID)
	}
	if err != nil {
		c.Log.Printf("derailing %s failed: %v", p.ID, err)
		return nil, err
	}
	return lostResult(lost), nil
}

func lostResult(lost []workspace.Unsaved) rpc.LostResult {
	var r rpc.LostResult
	for _, u := range lost {
		r.Lost = append(r.Lost, u.String())
	}
	return r
}

// autoArchive archives the old tracks and logs which.
func (c Config) autoArchive(ctx context.Context) error {
	names, err := c.Tracks.AutoArchive(ctx)
	if len(names) > 0 {
		c.Log.Printf("archived %s, ended over a week ago", strings.Join(names, ", "))
	}
	return err
}

func (c Config) create(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.CreateParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	created, err := c.Tracks.Create(ctx, p.Request, call.Progress)
	if err != nil {
		c.Log.Printf("creating a %s track failed: %v", p.Kind, err)
	} else {
		c.Log.Printf("created %s, %s", created.Track.ID, created.Track.Name)
	}
	select {
	case <-call.Gone:
		// The form was closed while the track was being made.
		if p.Client != "" {
			if err := c.Tmux.Tell(p.Client, c.outcome(created, err)); err != nil {
				c.Log.Printf("telling %s: %v", p.Client, err)
			}
		}
	default:
	}
	if err != nil {
		return nil, err
	}
	return rpc.CreateResult{ID: created.Track.ID, Name: created.Track.Name, Window: created.Window.ID}, nil
}

// outcome is how a creation went, for a status line.
func (c Config) outcome(created tracks.Created, err error) string {
	if err != nil {
		return "Couldn't create the track: " + err.Error()
	}
	msg := created.Track.Name + " is ready"
	if infos, err := c.Tracks.Windows.List(); err == nil {
		for _, in := range infos {
			if in.Window == created.Window.ID && in.Number <= 9 {
				msg += fmt.Sprintf(": Ctrl+b %d", in.Number)
			}
		}
	}
	return msg
}

// helpers installs what the prompts rely on: the reviewer subagents,
// the add-repo skill, the Cursor rule once Cursor is added, and a
// `tracks` on the tracks' PATH that runs this build.
func (c Config) helpers() {
	skipped, err := claude.InstallHelpers(c.Home)
	if err != nil {
		c.Log.Printf("installing the Claude helpers: %v", err)
	}
	if s, err := settings.Load(c.Paths.Settings); err != nil {
		c.Log.Printf("reading the settings for the Cursor rule: %v", err)
	} else if s.Engines.Cursor != nil {
		path, ok, err := cursor.InstallRule(c.Home)
		if err != nil {
			c.Log.Printf("installing the Cursor rule: %v", err)
		} else if !ok {
			skipped = append(skipped, path)
		}
	}
	for _, path := range skipped {
		c.Log.Printf("not overwriting %s: it lacks the x-tracks-managed marker, so it isn't Tracks'", path)
	}
	if err := writeShim(c.Paths.BinDir); err != nil {
		c.Log.Printf("writing the tracks command: %v", err)
	}
}

func writeShim(dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tracks-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	script := "#!/bin/sh\nexec " + shellx.Quote(exe) + " \"$@\"\n"
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "tracks"))
}
