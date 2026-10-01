package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Draft is a creation that failed, kept to start again. Request is
// what the form asked for, as JSON; Error why the last creation from it
// failed; FailedAt when it failed first.
type Draft struct {
	ID       string
	Request  string
	Error    string
	FailedAt time.Time
}

// SaveDraft adds d, or updates the draft with its ID: its request and
// error change, and it keeps when it failed first.
func (s *Store) SaveDraft(ctx context.Context, d Draft) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO drafts (id, request, error, failed_at) VALUES (?, ?, ?, ?) "+
		"ON CONFLICT (id) DO UPDATE SET request = excluded.request, error = excluded.error",
		d.ID, d.Request, d.Error, d.FailedAt.UnixMilli())
	return err
}

// Drafts are every draft, oldest first.
func (s *Store) Drafts(ctx context.Context) ([]Draft, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, request, error, failed_at FROM drafts ORDER BY failed_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Draft
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Draft is the draft id; ErrNotFound when there's none.
func (s *Store) Draft(ctx context.Context, id string) (Draft, error) {
	d, err := scanDraft(s.db.QueryRowContext(ctx, "SELECT id, request, error, failed_at FROM drafts WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	return d, err
}

// DeleteDraft removes the draft id. deleted is false when there was
// none.
func (s *Store) DeleteDraft(ctx context.Context, id string) (deleted bool, err error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func scanDraft(row interface{ Scan(...any) error }) (Draft, error) {
	var d Draft
	var failed int64
	if err := row.Scan(&d.ID, &d.Request, &d.Error, &failed); err != nil {
		return Draft{}, err
	}
	d.FailedAt = time.UnixMilli(failed)
	return d, nil
}
