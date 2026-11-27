package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SetAgentToken makes hash the token of track id's agent, in place of
// the one before: a new session's token. ErrNotFound when there's no
// track id.
func (s *Store) SetAgentToken(ctx context.Context, id, hash string) error {
	return s.updateTrack(ctx, id,
		"INSERT INTO agent_tokens (hash, track_id, holder, at) SELECT ?, id, '', ? FROM tracks WHERE id = ? "+
			"ON CONFLICT (track_id, holder) DO UPDATE SET hash = excluded.hash, at = excluded.at",
		hash, time.Now().UnixMilli(), id)
}

// AgentToken is the track whose agent holds the token with hash;
// ErrNotFound for none.
func (s *Store) AgentToken(ctx context.Context, hash string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT track_id FROM agent_tokens WHERE hash = ?", hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
