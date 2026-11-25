package store

import (
	"context"
	"time"
)

// Instruction is a prompt Tracks types into a track's agent once it's
// free: Kind is the event it answers, PR the pull request it's about.
type Instruction struct {
	ID             int64
	TrackID        string
	Kind, PR, Body string
	CreatedAt      time.Time
}

// AddInstruction queues body for track id's agent. One about the same
// kind and PR that still waits gives way to it: the newer says what's
// so now.
func (s *Store) AddInstruction(ctx context.Context, in Instruction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM instructions WHERE track_id = ? AND kind = ? AND pr_url = ? AND typed_at IS NULL",
		in.TrackID, in.Kind, in.PR); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO instructions (track_id, kind, pr_url, body, created_at) VALUES (?, ?, ?, ?, ?)",
		in.TrackID, in.Kind, in.PR, in.Body, in.CreatedAt.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// WaitingInstructions are the instructions not yet typed, oldest first.
func (s *Store) WaitingInstructions(ctx context.Context) ([]Instruction, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, track_id, kind, pr_url, body, created_at FROM instructions "+
		"WHERE typed_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Instruction
	for rows.Next() {
		var in Instruction
		var at int64
		if err := rows.Scan(&in.ID, &in.TrackID, &in.Kind, &in.PR, &in.Body, &at); err != nil {
			return nil, err
		}
		in.CreatedAt = time.UnixMilli(at)
		out = append(out, in)
	}
	return out, rows.Err()
}

// InstructionTyped marks instruction id typed at at.
func (s *Store) InstructionTyped(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE instructions SET typed_at = ? WHERE id = ?", at.UnixMilli(), id)
	return err
}
