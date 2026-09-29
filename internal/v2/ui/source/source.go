// Package source is where screens get their data from. Screens depend
// on Source only; Daemon reads the tracks from the daemon.
package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// Track is what screens show about a track.
type Track struct {
	ID string
	// Number is the track's window index, as the footer and keys use
	// it; 0 for an ended track, which has no window.
	Number int
	Name   string
	Kind   string
	Status track.Status
	// Cleanable says Clean can remove the worktrees: the track ended and
	// has some.
	Cleanable bool
	Repos     []Repo
	// Engine is the agent CLI, Model its model; Session is the agent's
	// session ID, which resumes it.
	Engine, Model, Session string
	Cost                   float64 // in US dollars, 0 when unknown
	PR                     *PR     // nil without a pull request
}

// Repo is one repository of a track; Path is its worktree, or the
// primary checkout for a track without worktrees. Removed says Clean
// removed the worktree; Path is empty then.
type Repo struct {
	Name    string
	Branch  string
	Path    string
	Removed bool
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

// Open reports whether t has a window.
func (t Track) Open() bool { return t.Status != track.Done && t.Status != track.Closed }

// Daemon reads the tracks from the daemon, with List.
type Daemon struct {
	List func(ctx context.Context) ([]tracks.Listed, error)
}

// Tracks lists the open tracks in window order, then the ended ones,
// most recently ended first.
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
			if l.Cleaned() && r.Worktree != "" {
				repos[j].Path, repos[j].Removed = "", true
			}
		}
		engine := l.Engine
		if e, ok := agents.ByID(l.Engine); ok {
			engine = e.Name
		}
		out[i] = Track{ID: l.ID, Number: l.Number, Name: l.Name, Kind: string(l.Kind), Status: l.Status(),
			Cleanable: !l.Open() && l.Kind.Worktrees() && !l.Cleaned(), Repos: repos,
			Engine: engine, Model: l.Model, Session: l.Session}
	}
	return out, nil
}
