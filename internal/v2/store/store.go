// Package store keeps Tracks' data in a SQLite database: schema,
// migrations and queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrNameTaken = errors.New("name already taken")
	ErrPathTaken = errors.New("path already taken")
	// ErrTooNew is returned for a database written by a newer Tracks.
	ErrTooNew = errors.New("database is newer than this Tracks")
)

// Store is an open database.
type Store struct {
	db *sql.DB
}

// Open opens the database at path, creating it if needed, and migrates
// it to the current schema.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	// One connection: writes never compete inside the process, and the
	// pragmas below hold for every query.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	all, err := steps()
	if err == nil {
		err = migrate(ctx, db, path, all)
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// dsn sets up every connection:
//   - WAL lets other processes read while one writes;
//   - busy_timeout waits for another process's write lock instead of
//     failing;
//   - synchronous=NORMAL is safe with WAL: a crash loses nothing, a
//     power cut at most the last transaction;
//   - foreign_keys is off per connection unless turned on;
//   - _txlock=immediate takes the write lock when a transaction
//     begins, so two writers wait instead of failing mid-transaction.
func dsn(path string) string {
	q := url.Values{}
	for _, p := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)", "foreign_keys(1)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	return (&url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}).String()
}

// constraint maps a unique-constraint failure to the error for its
// column, and leaves other errors alone.
func constraint(err error) error {
	var e *sqlite.Error
	if !errors.As(err, &e) || e.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return err
	}
	switch msg := e.Error(); {
	case strings.Contains(msg, ".name"):
		return ErrNameTaken
	case strings.Contains(msg, ".path"):
		return ErrPathTaken
	}
	return err
}
