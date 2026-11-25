package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
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
// it's new. was is the state it had, "" when it's new: it changed
// unless was is pr.State.
func (s *Store) SavePR(ctx context.Context, id string, pr track.PR, at time.Time) (was track.PRState, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT state FROM track_prs WHERE track_id = ? AND url = ?", id, pr.URL).Scan(&was)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	} else if err != nil {
		return "", err
	}
	if was == pr.State {
		_, err = s.db.ExecContext(ctx, "UPDATE track_prs SET checked_at = ? WHERE track_id = ? AND url = ?",
			millis(pr.CheckedAt), id, pr.URL)
		return was, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO track_prs (track_id, url, repo, number, state, found_at, checked_at) "+
		"VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (track_id, url) DO UPDATE SET state = excluded.state, checked_at = excluded.checked_at",
		id, pr.URL, pr.Repo, pr.Number, string(pr.State), at.UnixMilli(), millis(pr.CheckedAt))
	return was, err
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

// SavePRHealth records h as track id's PR at url has it now and returns
// what it had: zero until the poll read it. A Mergeable GitHub hasn't
// computed keeps the last one.
func (s *Store) SavePRHealth(ctx context.Context, id, url string, h track.Health) (was track.Health, err error) {
	var mergeable, checks, head sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT mergeable, checks, head FROM track_prs WHERE track_id = ? AND url = ?", id, url).
		Scan(&mergeable, &checks, &head)
	if errors.Is(err, sql.ErrNoRows) {
		return track.Health{}, ErrNotFound
	} else if err != nil {
		return track.Health{}, err
	}
	was = track.Health{Mergeable: track.Mergeable(mergeable.String), Checks: track.Checks(checks.String), Head: head.String}
	_, err = s.db.ExecContext(ctx, "UPDATE track_prs SET mergeable = COALESCE(?, mergeable), checks = ?, head = ? "+
		"WHERE track_id = ? AND url = ?", nullString(string(h.Mergeable)), nullString(string(h.Checks)), nullString(h.Head), id, url)
	return was, err
}

// SavePRThreads records n open review threads on track id's PR at url
// and returns how many it had; known is false until they were counted.
func (s *Store) SavePRThreads(ctx context.Context, id, url string, n int) (was int, known bool, err error) {
	var threads sql.NullInt64
	err = s.db.QueryRowContext(ctx, "SELECT threads FROM track_prs WHERE track_id = ? AND url = ?", id, url).Scan(&threads)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, ErrNotFound
	} else if err != nil {
		return 0, false, err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE track_prs SET threads = ? WHERE track_id = ? AND url = ?", n, id, url)
	return int(threads.Int64), threads.Valid, err
}

// nullString is s, NULL for "".
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
