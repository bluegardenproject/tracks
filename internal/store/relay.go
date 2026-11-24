package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bluegardenproject/tracks/internal/relay"
)

// AddRelayMessage adds m to track id's relay and returns it with its ID
// and time.
func (s *Store) AddRelayMessage(ctx context.Context, id string, m relay.Message) (relay.Message, error) {
	if m.At.IsZero() {
		m.At = time.Now()
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO relay_messages (track_id, sender, recipient, kind, body, at) VALUES (?, ?, ?, ?, ?, ?)",
		id, m.From, m.To, string(m.Kind), m.Body, m.At.UnixMilli())
	if err != nil {
		return relay.Message{}, err
	}
	m.ID, err = res.LastInsertId()
	return m, err
}

// RelayUnread are the messages on track id's relay for reader that it
// hasn't read, oldest first: to it or to everyone, or every one for the
// track, but none it sent.
func (s *Store) RelayUnread(ctx context.Context, id, reader string) ([]relay.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, sender, recipient, kind, body, at FROM relay_messages "+
			"WHERE track_id = ? AND id > COALESCE((SELECT last_id FROM relay_cursors WHERE track_id = ? AND reader = ?), 0) "+
			"AND sender != ? AND (? = ? OR recipient IN (?, '')) ORDER BY id",
		id, id, reader, reader, reader, relay.TrackHandle, reader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []relay.Message
	for rows.Next() {
		m, err := scanRelay(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkRelayRead records that reader read track id's relay up to message
// upTo; a reader never goes back. It reports whether the cursor moved.
func (s *Store) MarkRelayRead(ctx context.Context, id, reader string, upTo int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO relay_cursors (track_id, reader, last_id) VALUES (?, ?, ?) "+
			"ON CONFLICT (track_id, reader) DO UPDATE SET last_id = excluded.last_id WHERE excluded.last_id > last_id",
		id, reader, upTo)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RelayLog is every message on track id's relay, oldest first.
func (s *Store) RelayLog(ctx context.Context, id string) ([]relay.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, sender, recipient, kind, body, at FROM relay_messages WHERE track_id = ? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []relay.Message
	for rows.Next() {
		m, err := scanRelay(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RelayUnseen counts the messages on track id's relay past reader's
// cursor, whoever they're for.
func (s *Store) RelayUnseen(ctx context.Context, id, reader string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM relay_messages WHERE track_id = ? "+
			"AND id > COALESCE((SELECT last_id FROM relay_cursors WHERE track_id = ? AND reader = ?), 0)",
		id, id, reader).Scan(&n)
	return n, err
}

// RelayMessage is message msg of track id's relay; ErrNotFound when it
// has none such.
func (s *Store) RelayMessage(ctx context.Context, id string, msg int64) (relay.Message, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, sender, recipient, kind, body, at FROM relay_messages WHERE track_id = ? AND id = ?", id, msg)
	m, err := scanRelay(row)
	if errors.Is(err, sql.ErrNoRows) {
		return relay.Message{}, ErrNotFound
	}
	return m, err
}

func scanRelay(row interface{ Scan(...any) error }) (relay.Message, error) {
	var m relay.Message
	var kind string
	var at int64
	if err := row.Scan(&m.ID, &m.From, &m.To, &kind, &m.Body, &at); err != nil {
		return relay.Message{}, err
	}
	m.Kind, m.At = relay.Kind(kind), time.UnixMilli(at)
	return m, nil
}
