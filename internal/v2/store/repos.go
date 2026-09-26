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
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const repoColumns = "id, name, path, base_branch, draft_prs, created_at, updated_at"

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
	return out, rows.Err()
}

// Repo returns the repo with id.
func (s *Store) Repo(ctx context.Context, id int64) (Repo, error) {
	r, err := scanRepo(s.db.QueryRowContext(ctx, "SELECT "+repoColumns+" FROM repos WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Repo{}, ErrNotFound
	}
	return r, err
}

// AddRepo stores r as a new repo and returns it with its ID and times.
func (s *Store) AddRepo(ctx context.Context, r Repo) (Repo, error) {
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO repos (name, path, base_branch, draft_prs, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		r.Name, r.Path, r.BaseBranch, r.DraftPRs, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Repo{}, constraint(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Repo{}, err
	}
	return s.Repo(ctx, id)
}

// UpdateRepo stores r's fields under its ID and returns the result.
func (s *Store) UpdateRepo(ctx context.Context, r Repo) (Repo, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE repos SET name = ?, path = ?, base_branch = ?, draft_prs = ?, updated_at = ? WHERE id = ?",
		r.Name, r.Path, r.BaseBranch, r.DraftPRs, time.Now().UnixMilli(), r.ID)
	if err != nil {
		return Repo{}, constraint(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Repo{}, err
	} else if n == 0 {
		return Repo{}, ErrNotFound
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
	err := row.Scan(&r.ID, &r.Name, &r.Path, &r.BaseBranch, &r.DraftPRs, &created, &updated)
	r.CreatedAt, r.UpdatedAt = time.UnixMilli(created), time.UnixMilli(updated)
	return r, err
}
