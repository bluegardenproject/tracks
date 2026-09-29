package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/v2/agents/claude"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
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
		rpc.Clean:   c.clean,
		rpc.Archive: c.archive,
		rpc.Derail:  c.derail,
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

func (c Config) clean(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.CleanParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	var unsaved []workspace.Unsaved
	var err error
	if p.Check {
		unsaved, err = c.Tracks.Unsaved(ctx, p.ID)
	} else if unsaved, err = c.Tracks.Clean(ctx, p.ID, p.Force); err == nil && len(unsaved) == 0 {
		c.Log.Printf("cleaned %s", p.ID)
	}
	if err != nil {
		c.Log.Printf("cleaning %s failed: %v", p.ID, err)
		return nil, err
	}
	var r rpc.CleanResult
	for _, u := range unsaved {
		r.Unsaved = append(r.Unsaved, u.String())
	}
	return r, nil
}

func (c Config) archive(ctx context.Context, call *rpc.Call) (any, error) {
	var p rpc.ArchiveParams
	if err := call.Decode(&p); err != nil {
		return nil, err
	}
	unsaved, err := c.Tracks.Archive(ctx, p.ID, p.Force)
	if err != nil {
		c.Log.Printf("archiving %s failed: %v", p.ID, err)
		return nil, err
	}
	var r rpc.CleanResult
	for _, u := range unsaved {
		r.Unsaved = append(r.Unsaved, u.String())
	}
	if len(r.Unsaved) == 0 {
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
	var r rpc.CleanResult
	for _, u := range lost {
		r.Unsaved = append(r.Unsaved, u.String())
	}
	return r, nil
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
// and a `tracks` on the tracks' PATH that runs this build's v2 app.
func (c Config) helpers() {
	skipped, err := claude.InstallReviewers(c.Home)
	if err != nil {
		c.Log.Printf("installing the reviewer subagents: %v", err)
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
	script := "#!/bin/sh\nexec " + shellx.Quote(exe) + " --new-app \"$@\"\n"
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
