// Package source is where screens get their data from. Screens depend
// on Source only; Daemon reads the tracks from the daemon.
package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// Track is what screens show about a track.
type Track struct {
	Number int // the track's window index, as the footer and keys use it
	Name   string
	Kind   string
	Status string
	Repos  []Repo
	// Engine is the agent CLI, Model its model; Session is the agent's
	// session ID, which resumes it.
	Engine, Model, Session string
	Cost                   float64 // in US dollars, 0 when unknown
	PR                     *PR     // nil without a pull request
}

// Repo is one repository of a track; Path is its worktree, or the
// primary checkout for a track without worktrees.
type Repo struct {
	Name   string
	Branch string
	Path   string
}

// PR is a track's pull request.
type PR struct {
	Number int
	State  string // open, draft, merged, closed
	URL    string
}

// Source lists the tracks.
type Source interface {
	Tracks(ctx context.Context) ([]Track, error)
}

// Running is the status every track has until the status model exists.
const Running = "running"

// Daemon reads the open tracks from the daemon, with List.
type Daemon struct {
	List func(ctx context.Context) ([]tracks.Listed, error)
}

// Tracks lists the open tracks in window order.
func (d Daemon) Tracks(ctx context.Context) ([]Track, error) {
	listed, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Track, len(listed))
	for i, l := range listed {
		repos := make([]Repo, len(l.Repos))
		for j, r := range l.Repos {
			repos[j] = Repo{Name: r.Name, Branch: r.Branch, Path: r.Dir()}
		}
		engine := l.Engine
		if e, ok := agents.ByID(l.Engine); ok {
			engine = e.Name
		}
		out[i] = Track{Number: l.Number, Name: l.Name, Kind: string(l.Kind), Status: Running, Repos: repos,
			Engine: engine, Model: l.Model, Session: l.Session}
	}
	return out, nil
}
