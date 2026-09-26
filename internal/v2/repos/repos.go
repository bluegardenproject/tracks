// Package repos adds, changes and removes the repositories tracks are
// started in, checking each change against git and the running tracks.
package repos

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/store"
)

// Store is where repos are kept.
type Store interface {
	Repos(ctx context.Context) ([]store.Repo, error)
	Repo(ctx context.Context, id int64) (store.Repo, error)
	AddRepo(ctx context.Context, r store.Repo) (store.Repo, error)
	UpdateRepo(ctx context.Context, r store.Repo) (store.Repo, error)
	DeleteRepo(ctx context.Context, id int64) error
}

// Use is a running track in a repo.
type Use struct {
	Repo  string // the repo's name
	Track string
}

// UsesFunc lists the running tracks' repos.
type UsesFunc func(ctx context.Context) ([]Use, error)

// Service applies the rules for repos. Uses may be nil: then no track
// counts as using a repo.
type Service struct {
	Store Store
	Git   Git
	Uses  UsesFunc
}

// Entry is a repo with what the Repositories tab shows about it.
type Entry struct {
	store.Repo
	Remote string   // origin's web address, "" without one
	Tracks []string // running tracks in the repo
}

// FieldError is a problem with one field of a repo.
type FieldError struct {
	Field   string // "name", "path" or "base"
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// ErrInUse is returned for changes running tracks don't allow.
var ErrInUse = errors.New("running tracks use this repo")

// List returns every repo by name.
func (s Service) List(ctx context.Context) ([]Entry, error) {
	all, err := s.Store.Repos(ctx)
	if err != nil {
		return nil, err
	}
	uses, err := s.uses(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(all))
	for i, r := range all {
		entries[i] = Entry{Repo: r, Remote: s.remote(ctx, r.Path), Tracks: tracksOf(uses, r.Name)}
	}
	return entries, nil
}

// Add checks r and stores it as a new repo.
func (s Service) Add(ctx context.Context, r store.Repo) (store.Repo, error) {
	r, err := s.check(ctx, r)
	if err != nil {
		return store.Repo{}, err
	}
	added, err := s.Store.AddRepo(ctx, r)
	return added, taken(err)
}

// Update checks r and stores it over the repo with its ID. Name and
// path can't change while running tracks use the repo.
func (s Service) Update(ctx context.Context, r store.Repo) (store.Repo, error) {
	old, err := s.Store.Repo(ctx, r.ID)
	if err != nil {
		return store.Repo{}, err
	}
	r, err = s.check(ctx, r)
	if err != nil {
		return store.Repo{}, err
	}
	if r.Name != old.Name || r.Path != old.Path {
		if err := s.idle(ctx, old.Name); err != nil {
			return store.Repo{}, fmt.Errorf("can't change the name or path: %w", err)
		}
	}
	updated, err := s.Store.UpdateRepo(ctx, r)
	return updated, taken(err)
}

// Delete removes the repo with id, unless running tracks use it.
func (s Service) Delete(ctx context.Context, id int64) error {
	r, err := s.Store.Repo(ctx, id)
	if err != nil {
		return err
	}
	if err := s.idle(ctx, r.Name); err != nil {
		return fmt.Errorf("can't delete %s: %w", r.Name, err)
	}
	return s.Store.DeleteRepo(ctx, id)
}

func (s Service) uses(ctx context.Context) ([]Use, error) {
	if s.Uses == nil {
		return nil, nil
	}
	return s.Uses(ctx)
}

// idle fails when running tracks use the repo called name.
func (s Service) idle(ctx context.Context, name string) error {
	uses, err := s.uses(ctx)
	if err != nil {
		return err
	}
	if tracks := tracksOf(uses, name); len(tracks) > 0 {
		return fmt.Errorf("%w (%s)", ErrInUse, strings.Join(tracks, ", "))
	}
	return nil
}

func tracksOf(uses []Use, name string) []string {
	var tracks []string
	for _, u := range uses {
		if strings.EqualFold(u.Repo, name) {
			tracks = append(tracks, u.Track)
		}
	}
	slices.Sort(tracks)
	return tracks
}

// taken turns the store's uniqueness errors into field errors.
func taken(err error) error {
	switch {
	case errors.Is(err, store.ErrNameTaken):
		return &FieldError{"name", "Another repo already has this name."}
	case errors.Is(err, store.ErrPathTaken):
		return &FieldError{"path", "This checkout is already a repo."}
	}
	return err
}
