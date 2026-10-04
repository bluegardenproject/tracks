package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Repo is a repository tracks are started in.
type Repo struct {
	ID         int64
	Name       string
	Path       string // the primary checkout
	BaseBranch string
	DraftPRs   bool
	Setup      string // the command run once in a worktree before its dev servers
	Servers    []DevServer
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const repoColumns = "id, name, path, base_branch, draft_prs, setup, created_at, updated_at"

// Repos lists the repos by name.
func (s *Store) Repos(ctx context.Context) ([]Repo, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+repoColumns+" FROM repos ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repo
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	servers, err := s.devServers(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Servers = servers[out[i].ID]
	}
	return out, nil
}

// Repo returns the repo with id.
func (s *Store) Repo(ctx context.Context, id int64) (Repo, error) {
	r, err := scanRepo(s.db.QueryRowContext(ctx, "SELECT "+repoColumns+" FROM repos WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Repo{}, ErrNotFound
	}
	if err != nil {
		return Repo{}, err
	}
	servers, err := s.devServers(ctx, "WHERE repo_id = ?", id)
	r.Servers = servers[id]
	return r, err
}

// AddRepo stores r and its dev servers as a new repo and returns it
// with its ID and times.
func (s *Store) AddRepo(ctx context.Context, r Repo) (Repo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Repo{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO repos (name, path, base_branch, draft_prs, setup, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		r.Name, r.Path, r.BaseBranch, r.DraftPRs, r.Setup, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Repo{}, constraint(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Repo{}, err
	}
	if err := putDevServers(ctx, tx, id, r.Servers); err != nil {
		return Repo{}, err
	}
	if err := tx.Commit(); err != nil {
		return Repo{}, err
	}
	return s.Repo(ctx, id)
}

// UpdateRepo stores r's fields under its ID, replacing its dev servers,
// and returns the result.
func (s *Store) UpdateRepo(ctx context.Context, r Repo) (Repo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Repo{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE repos SET name = ?, path = ?, base_branch = ?, draft_prs = ?, setup = ?, updated_at = ? WHERE id = ?",
		r.Name, r.Path, r.BaseBranch, r.DraftPRs, r.Setup, time.Now().UnixMilli(), r.ID)
	if err != nil {
		return Repo{}, constraint(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Repo{}, err
	} else if n == 0 {
		return Repo{}, ErrNotFound
	}
	if err := putDevServers(ctx, tx, r.ID, r.Servers); err != nil {
		return Repo{}, err
	}
	if err := tx.Commit(); err != nil {
		return Repo{}, err
	}
	return s.Repo(ctx, r.ID)
}

// DeleteRepo removes the repo with id.
func (s *Store) DeleteRepo(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM repos WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanRepo(row scanner) (Repo, error) {
	var r Repo
	var created, updated int64
	err := row.Scan(&r.ID, &r.Name, &r.Path, &r.BaseBranch, &r.DraftPRs, &r.Setup, &created, &updated)
	r.CreatedAt, r.UpdatedAt = time.UnixMilli(created), time.UnixMilli(updated)
	return r, err
}
