package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

const trackColumns = "id, kind, name, engine, model, session_id, prompt, review_ref, document, " +
	"candor, opinion, claim_check, terminal, created_at, closed_at, cleaned_at, waiting, archived_at, title, cost"

// AddTrack stores t and its repos in one transaction.
func (s *Store) AddTrack(ctx context.Context, t track.Track) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "INSERT INTO tracks ("+trackColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		t.ID, string(t.Kind), t.Name, t.Engine, t.Model, t.Session, t.Prompt, t.ReviewRef, t.Document,
		t.Candor, t.Opinion, t.ClaimCheck, t.Terminal, t.CreatedAt.UnixMilli(), millis(t.ClosedAt), millis(t.CleanedAt), t.Waiting, millis(t.ArchivedAt), t.Title, t.Cost)
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
// the most recently closed first, without the archived ones.
func (s *Store) EndedTracks(ctx context.Context, limit int) ([]track.Track, error) {
	return s.tracks(ctx, "WHERE closed_at IS NOT NULL AND archived_at IS NULL ORDER BY closed_at DESC, created_at DESC LIMIT ?", limit)
}

// EndedBefore are the tracks not archived whose window closed before
// at, the longest closed first.
func (s *Store) EndedBefore(ctx context.Context, at time.Time) ([]track.Track, error) {
	return s.tracks(ctx, "WHERE closed_at < ? AND archived_at IS NULL ORDER BY closed_at, created_at", at.UnixMilli())
}

// SetState records st as id's state.
func (s *Store) SetState(ctx context.Context, id string, st track.State) error {
	return s.updateTrack(ctx, id, "UPDATE tracks SET closed_at = ?, cleaned_at = ?, waiting = ?, archived_at = ? WHERE id = ?",
		millis(st.ClosedAt), millis(st.CleanedAt), st.Waiting, millis(st.ArchivedAt), id)
}

// Rename records that id's window is now called name.
func (s *Store) Rename(ctx context.Context, id, name string) error {
	return s.updateTrack(ctx, id, "UPDATE tracks SET name = ? WHERE id = ?", name, id)
}

// SetCost records what id has cost so far.
func (s *Store) SetCost(ctx context.Context, id string, cost float64) error {
	return s.updateTrack(ctx, id, "UPDATE tracks SET cost = ? WHERE id = ?", cost, id)
}

// SetBranch records that track id's repo at position is on branch.
func (s *Store) SetBranch(ctx context.Context, id string, position int, branch string) error {
	return s.updateTrack(ctx, id, "UPDATE track_repos SET branch = ? WHERE track_id = ? AND position = ?", branch, id, position)
}

// AddTrackRepo adds r to track id's repos, after the others; ErrNotFound
// when there's no track id.
func (s *Store) AddTrackRepo(ctx context.Context, id string, r track.Repo) error {
	var repoID any
	if r.RepoID != 0 {
		repoID = r.RepoID
	}
	return s.updateTrack(ctx, id,
		"INSERT INTO track_repos (track_id, position, repo_id, name, path, worktree, branch, base) "+
			"SELECT id, (SELECT COALESCE(MAX(position) + 1, 0) FROM track_repos WHERE track_id = ?), ?, ?, ?, ?, ?, ? FROM tracks WHERE id = ?",
		id, repoID, r.Name, r.Path, r.Worktree, r.Branch, r.Base, id)
}

// DeleteTrack deletes track id with its repos and PRs; ErrNotFound when
// there's none.
func (s *Store) DeleteTrack(ctx context.Context, id string) error {
	return s.updateTrack(ctx, id, "DELETE FROM tracks WHERE id = ?", id)
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
	if err := s.addRepos(ctx, out, index); err != nil {
		return nil, err
	}
	return out, s.addPRs(ctx, out, index)
}

// idList is the IDs of tracks as query arguments, and their
// placeholders.
func idList(tracks []track.Track) ([]any, string) {
	ids := make([]any, 0, len(tracks))
	marks := make([]byte, 0, 2*len(tracks))
	for _, t := range tracks {
		ids = append(ids, t.ID)
		if len(marks) > 0 {
			marks = append(marks, ',')
		}
		marks = append(marks, '?')
	}
	return ids, string(marks)
}

func (s *Store) addRepos(ctx context.Context, tracks []track.Track, index map[string]int) error {
	ids, marks := idList(tracks)
	rows, err := s.db.QueryContext(ctx, "SELECT track_id, repo_id, name, path, worktree, branch, base FROM track_repos "+
		"WHERE track_id IN ("+marks+") ORDER BY track_id, position", ids...)
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
	var closed, cleaned, archived sql.NullInt64
	err := row.Scan(&t.ID, &kind, &t.Name, &t.Engine, &t.Model, &t.Session, &t.Prompt, &t.ReviewRef, &t.Document,
		&t.Candor, &t.Opinion, &t.ClaimCheck, &t.Terminal, &created, &closed, &cleaned, &t.Waiting, &archived, &t.Title, &t.Cost)
	t.Kind = track.Kind(kind)
	t.CreatedAt = time.UnixMilli(created)
	if closed.Valid {
		t.ClosedAt = time.UnixMilli(closed.Int64)
	}
	if cleaned.Valid {
		t.CleanedAt = time.UnixMilli(cleaned.Int64)
	}
	if archived.Valid {
		t.ArchivedAt = time.UnixMilli(archived.Int64)
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
