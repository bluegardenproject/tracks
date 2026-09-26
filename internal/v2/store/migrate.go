package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// migrations are applied in file name order; the database's
// user_version counts how many have been.
//
//go:embed migrations/*.sql
var migrations embed.FS

// keepBackups is how many backups before migrations are kept.
const keepBackups = 3

// migrate applies the steps the database at path hasn't had yet.
func migrate(ctx context.Context, db *sql.DB, path string, steps []string) error {
	version, err := userVersion(ctx, db)
	if err != nil {
		return err
	}
	if version > len(steps) {
		return fmt.Errorf("%w: schema %d, this Tracks knows %d", ErrTooNew, version, len(steps))
	}
	if version == len(steps) {
		return nil
	}
	if version > 0 {
		if err := backup(ctx, db, path, version); err != nil {
			return fmt.Errorf("backup before migrating: %w", err)
		}
	}
	for v := version; v < len(steps); v++ {
		if err := step(ctx, db, v, steps[v]); err != nil {
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
	}
	return nil
}

// step applies migration number v+1, unless another process already
// did while this one waited for the write lock.
func step(ctx context.Context, db *sql.DB, v int, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > v {
		return nil
	}
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
		return err
	}
	return tx.Commit()
}

func steps() ([]string, error) {
	names, err := migrations.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		b, err := migrations.ReadFile("migrations/" + n.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, string(b))
	}
	return out, nil
}

func userVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v int
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

// backup writes a consistent copy of the database next to it, which a
// plain file copy isn't while the WAL holds recent writes, and keeps
// the newest few.
func backup(ctx context.Context, db *sql.DB, path string, version int) error {
	to := fmt.Sprintf("%s.%s.v%d.bak", path, time.Now().UTC().Format("20060102T150405.000Z"), version)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", to); err != nil {
		return err
	}
	old, err := filepath.Glob(path + ".*.bak")
	if err != nil {
		return err
	}
	slices.Sort(old)
	for len(old) > keepBackups {
		_ = os.Remove(old[0])
		old = old[1:]
	}
	return nil
}
