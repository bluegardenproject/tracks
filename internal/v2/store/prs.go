package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// TrackPR is a pull request and the track it belongs to.
type TrackPR struct {
	TrackID string
	track.PR
}

// AddPR records pr as track id's, found at at, unless it's known: a
// hook seeing a PR again doesn't undo what the poll found.
func (s *Store) AddPR(ctx context.Context, id string, pr track.PR, at time.Time) (added bool, err error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO track_prs (track_id, url, repo, number, state, found_at, checked_at) "+
		"VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (track_id, url) DO NOTHING",
		id, pr.URL, pr.Repo, pr.Number, string(pr.State), at.UnixMilli(), millis(pr.CheckedAt))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SavePR records pr as GitHub has it now, adding it to track id's when
// it's new. changed is whether it's new or its state is.
func (s *Store) SavePR(ctx context.Context, id string, pr track.PR, at time.Time) (changed bool, err error) {
	res, err := s.db.ExecContext(ctx, "UPDATE track_prs SET checked_at = ? WHERE track_id = ? AND url = ? AND state = ?",
		millis(pr.CheckedAt), id, pr.URL, string(pr.State))
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return false, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO track_prs (track_id, url, repo, number, state, found_at, checked_at) "+
		"VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (track_id, url) DO UPDATE SET state = excluded.state, checked_at = excluded.checked_at",
		id, pr.URL, pr.Repo, pr.Number, string(pr.State), at.UnixMilli(), millis(pr.CheckedAt))
	return err == nil, err
}

// UnsettledPRs are the PRs still open or in draft, of every track.
func (s *Store) UnsettledPRs(ctx context.Context) ([]TrackPR, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT track_id, url, repo, number, state, checked_at FROM track_prs "+
		"WHERE state IN ('open', 'draft') ORDER BY track_id, found_at, url")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackPR
	for rows.Next() {
		var p TrackPR
		if p.PR, err = scanPR(rows, &p.TrackID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// addPRs reads the PRs of tracks, found by their IDs in index.
func (s *Store) addPRs(ctx context.Context, tracks []track.Track, index map[string]int) error {
	ids, marks := idList(tracks)
	rows, err := s.db.QueryContext(ctx, "SELECT track_id, url, repo, number, state, checked_at FROM track_prs "+
		"WHERE track_id IN ("+marks+") ORDER BY track_id, found_at, url", ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		pr, err := scanPR(rows, &id)
		if err != nil {
			return err
		}
		i := index[id]
		tracks[i].PRs = append(tracks[i].PRs, pr)
	}
	return rows.Err()
}

func scanPR(row scanner, id *string) (track.PR, error) {
	var pr track.PR
	var state string
	var checked sql.NullInt64
	err := row.Scan(id, &pr.URL, &pr.Repo, &pr.Number, &state, &checked)
	pr.State = track.PRState(state)
	if checked.Valid {
		pr.CheckedAt = time.UnixMilli(checked.Int64)
	}
	return pr, err
}
