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
		"SELECT m.id, m.sender, m.recipient, m.kind, m.body, m.at FROM relay_messages m "+
			"LEFT JOIN relay_cursors c ON c.track_id = m.track_id AND c.reader = ? "+
			"WHERE m.track_id = ? AND m.id > COALESCE(c.last_id, 0) AND m.sender != ? "+
			"AND (? = ? OR m.recipient IN (?, '')) ORDER BY m.id",
		reader, id, reader, reader, relay.TrackHandle, reader)
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
// upTo; a reader never goes back.
func (s *Store) MarkRelayRead(ctx context.Context, id, reader string, upTo int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO relay_cursors (track_id, reader, last_id) VALUES (?, ?, ?) "+
			"ON CONFLICT (track_id, reader) DO UPDATE SET last_id = max(last_id, excluded.last_id)",
		id, reader, upTo)
	return err
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
