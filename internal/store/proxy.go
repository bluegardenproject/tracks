package store

import (
	"context"
	"errors"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrPortTaken means the proxy already has the output port.
var ErrPortTaken = errors.New("output port already added")

// ProxyInput is what an output port forwards to: a dev server by track,
// repo and name, or a server found in a track by its port. The zero
// value is none.
type ProxyInput struct {
	Track  string `json:"track,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Server string `json:"server,omitempty"`
	Port   int    `json:"port,omitempty"`
}

// None reports whether in is no input.
func (in ProxyInput) None() bool { return in.Track == "" }

// ProxyPort is an output port and its input.
type ProxyPort struct {
	Port  int        `json:"port"`
	Input ProxyInput `json:"input"`
}

// ProxyPorts lists the output ports in the order they were added.
func (s *Store) ProxyPorts(ctx context.Context) ([]ProxyPort, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT port, input_track, input_repo, input_server, input_port FROM proxy_ports ORDER BY created_at, port")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProxyPort
	for rows.Next() {
		var p ProxyPort
		if err := rows.Scan(&p.Port, &p.Input.Track, &p.Input.Repo, &p.Input.Server, &p.Input.Port); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddProxyPort adds output port without an input.
func (s *Store) AddProxyPort(ctx context.Context, port int) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO proxy_ports (port, created_at) VALUES (?, ?)", port, time.Now().UnixMilli())
	var e *sqlite.Error
	if errors.As(err, &e) && (e.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY || e.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE) {
		return ErrPortTaken
	}
	return err
}

// RemoveProxyPort removes output port.
func (s *Store) RemoveProxyPort(ctx context.Context, port int) error {
	return s.changeOne(ctx, "DELETE FROM proxy_ports WHERE port = ?", port)
}

// SetProxyInput sets what output port forwards to.
func (s *Store) SetProxyInput(ctx context.Context, port int, in ProxyInput) error {
	return s.changeOne(ctx, "UPDATE proxy_ports SET input_track = ?, input_repo = ?, input_server = ?, input_port = ? WHERE port = ?",
		in.Track, in.Repo, in.Server, in.Port, port)
}

// changeOne runs query, which should change one row: ErrNotFound when
// it changed none.
func (s *Store) changeOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
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
