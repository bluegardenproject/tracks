package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNoPorts means every block of ports is held by an open track.
var ErrNoPorts = errors.New("no free block of ports")

// ClaimPorts returns track id's block of ports: the one it holds, or
// the lowest free one of blocks, each size wide from first. Blocks of
// ended tracks are free.
func (s *Store) ClaimPorts(ctx context.Context, id string, first, size, blocks int) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var base int
	err = tx.QueryRowContext(ctx, "SELECT base FROM track_ports WHERE track_id = ?", id).Scan(&base)
	if err == nil {
		return base, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM track_ports WHERE track_id IN (SELECT id FROM tracks WHERE closed_at IS NOT NULL)"); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT base FROM track_ports")
	if err != nil {
		return 0, err
	}
	taken := map[int]bool{}
	for rows.Next() {
		var b int
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return 0, err
		}
		taken[b] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for i := range blocks {
		if b := first + i*size; !taken[b] {
			if _, err := tx.ExecContext(ctx, "INSERT INTO track_ports (track_id, base) VALUES (?, ?)", id, b); err != nil {
				return 0, err
			}
			return b, tx.Commit()
		}
	}
	return 0, ErrNoPorts
}
