package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/repos"
	"github.com/bluegardenproject/tracks/internal/v2/store"
)

type (
	// Repository is a stored repository.
	Repository = store.Repo
	// RepositoryEntry is a repository with its remote and running
	// tracks.
	RepositoryEntry = repos.Entry
	// FieldError is a problem with one field of a repo.
	FieldError = repos.FieldError
)

// DefaultBase is the base branch of a repo saved without one.
const DefaultBase = repos.DefaultBase

// Repos manages the repositories; repos.Service is one.
type Repos interface {
	List(ctx context.Context) ([]RepositoryEntry, error)
	// Suggest proposes a repository for the checkout at path.
	Suggest(ctx context.Context, path string) (RepositoryEntry, error)
	Add(ctx context.Context, r Repository) (Repository, error)
	Update(ctx context.Context, r Repository) (Repository, error)
	Delete(ctx context.Context, id int64) error
}

var _ Repos = repos.Service{}
