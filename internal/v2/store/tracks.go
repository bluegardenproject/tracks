package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

const trackColumns = "id, kind, name, engine, model, session_id, prompt, review_ref, document, " +
	"candor, opinion, claim_check, terminal, created_at, closed_at, cleaned_at"

// AddTrack stores t and its repos in one transaction.
func (s *Store) AddTrack(ctx context.Context, t track.Track) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "INSERT INTO tracks ("+trackColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		t.ID, string(t.Kind), t.Name, t.Engine, t.Model, t.Session, t.Prompt, t.ReviewRef, t.Document,
		t.Candor, t.Opinion, t.ClaimCheck, t.Terminal, t.CreatedAt.UnixMilli(), millis(t.ClosedAt), millis(t.CleanedAt))
	if err != nil {
		return err
	}
	for i, r := range t.Repos {
		var repoID any
		if r.RepoID != 0 {
			repoID = r.RepoID
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO track_repos (track_id, position, repo_id, name, path, worktree, branch, base) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			t.ID, i, repoID, r.Name, r.Path, r.Worktree, r.Branch, r.Base); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Track returns the track with id.
func (s *Store) Track(ctx context.Context, id string) (track.Track, error) {
	tracks, err := s.tracks(ctx, "WHERE id = ?", id)
	if err != nil {
		return track.Track{}, err
	}
	if len(tracks) == 0 {
		return track.Track{}, ErrNotFound
	}
	return tracks[0], nil
}

// OpenTracks are the tracks whose window is open, oldest first.
func (s *Store) OpenTracks(ctx context.Context) ([]track.Track, error) {
	return s.tracks(ctx, "WHERE closed_at IS NULL ORDER BY created_at, id")
}

// EndedTracks are the last tracks whose window closed, at most limit,
// the most recently closed first.
func (s *Store) EndedTracks(ctx context.Context, limit int) ([]track.Track, error) {
	return s.tracks(ctx, "WHERE closed_at IS NOT NULL ORDER BY closed_at DESC, created_at DESC LIMIT ?", limit)
}

// ReopenTrack records that id's window is open again, under name, with
// its worktrees back.
func (s *Store) ReopenTrack(ctx context.Context, id, name string) error {
	return s.updateTrack(ctx, id, "UPDATE tracks SET closed_at = NULL, cleaned_at = NULL, name = ? WHERE id = ?", name, id)
}

// CleanTrack records that id's worktrees were removed at at.
func (s *Store) CleanTrack(ctx context.Context, id string, at time.Time) error {
	return s.updateTrack(ctx, id, "UPDATE tracks SET cleaned_at = ? WHERE id = ?", at.UnixMilli(), id)
}

// updateTrack runs query on id's row; ErrNotFound when there's none.
func (s *Store) updateTrack(ctx context.Context, id, query string, args ...any) error {
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

// CloseTrack records that id's window closed at at. A track already
// closed keeps its time.
func (s *Store) CloseTrack(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, "UPDATE tracks SET closed_at = ? WHERE id = ? AND closed_at IS NULL", at.UnixMilli(), id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		if _, err := s.Track(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// tracks runs a query on tracks with where, then reads their repos in
// one more query.
func (s *Store) tracks(ctx context.Context, where string, args ...any) ([]track.Track, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+trackColumns+" FROM tracks "+where, args...)
	if err != nil {
		return nil, err
	}
	var out []track.Track
	index := map[string]int{}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[t.ID] = len(out)
		out = append(out, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	return out, s.addRepos(ctx, out, index)
}

func (s *Store) addRepos(ctx context.Context, tracks []track.Track, index map[string]int) error {
	ids := make([]any, 0, len(tracks))
	marks := make([]byte, 0, 2*len(tracks))
	for _, t := range tracks {
		ids = append(ids, t.ID)
		if len(marks) > 0 {
			marks = append(marks, ',')
		}
		marks = append(marks, '?')
	}
	rows, err := s.db.QueryContext(ctx, "SELECT track_id, repo_id, name, path, worktree, branch, base FROM track_repos "+
		"WHERE track_id IN ("+string(marks)+") ORDER BY track_id, position", ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var repoID sql.NullInt64
		var r track.Repo
		if err := rows.Scan(&id, &repoID, &r.Name, &r.Path, &r.Worktree, &r.Branch, &r.Base); err != nil {
			return err
		}
		r.RepoID = repoID.Int64
		i := index[id]
		tracks[i].Repos = append(tracks[i].Repos, r)
	}
	return rows.Err()
}

func scanTrack(row scanner) (track.Track, error) {
	var t track.Track
	var kind string
	var created int64
	var closed, cleaned sql.NullInt64
	err := row.Scan(&t.ID, &kind, &t.Name, &t.Engine, &t.Model, &t.Session, &t.Prompt, &t.ReviewRef, &t.Document,
		&t.Candor, &t.Opinion, &t.ClaimCheck, &t.Terminal, &created, &closed, &cleaned)
	t.Kind = track.Kind(kind)
	t.CreatedAt = time.UnixMilli(created)
	if closed.Valid {
		t.ClosedAt = time.UnixMilli(closed.Int64)
	}
	if cleaned.Valid {
		t.CleanedAt = time.UnixMilli(cleaned.Int64)
	}
	return t, err
}

// millis is t in Unix milliseconds, NULL for the zero time.
func millis(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMilli()
}
