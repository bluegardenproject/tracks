// Package source is where screens get their data from. Screens depend
// on Source only; until the daemon exists, Windows reads the tracks
// from their tmux windows.
package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
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

// Repo is one repository of a track, checked out in a worktree.
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

// Windows reads the tracks of a tmux session from their windows, which
// know the track's kind, repo and worktree.
type Windows struct {
	Tmux    trackwin.Tmux
	Session string
}

// Tracks lists the session's tracks in window order.
func (w Windows) Tracks(context.Context) ([]Track, error) {
	infos, err := trackwin.List(w.Tmux, w.Session)
	if err != nil {
		return nil, err
	}
	tracks := make([]Track, len(infos))
	for i, in := range infos {
		tracks[i] = Track{Number: in.Number, Name: in.Name, Kind: in.Kind, Status: Running,
			Repos: []Repo{{Name: in.Repo, Path: in.Dir}}}
	}
	return tracks, nil
}
