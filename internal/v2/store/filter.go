package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Filter is Station's filter; the zero Filter when none is on.
func (s *Store) Filter(ctx context.Context) (track.Filter, error) {
	var f track.Filter
	var statuses, prStatuses, started string
	err := s.db.QueryRowContext(ctx, "SELECT statuses, pr_statuses, archived, started, from_date, to_date FROM station_filter WHERE id = 1").
		Scan(&statuses, &prStatuses, &f.Archived, &started, &f.From, &f.To)
	if errors.Is(err, sql.ErrNoRows) {
		return track.Filter{}, nil
	}
	f.Statuses, f.PRStatuses, f.Started = split(statuses), split(prStatuses), track.Started(started)
	return f, err
}

// SetFilter keeps f as Station's filter; the zero Filter clears it.
func (s *Store) SetFilter(ctx context.Context, f track.Filter) error {
	if !f.On() {
		_, err := s.db.ExecContext(ctx, "DELETE FROM station_filter")
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO station_filter (id, statuses, pr_statuses, archived, started, from_date, to_date)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET statuses = excluded.statuses, pr_statuses = excluded.pr_statuses,
			archived = excluded.archived, started = excluded.started, from_date = excluded.from_date, to_date = excluded.to_date`,
		strings.Join(f.Statuses, ","), strings.Join(f.PRStatuses, ","), f.Archived, string(f.Started), f.From, f.To)
	return err
}

// statusSQL is the condition for each track status, as State.Status
// derives it.
var statusSQL = map[string]string{
	track.Error.ID:          "(closed_at IS NULL AND agent_exit = 'failed')",
	track.ActionRequired.ID: "(closed_at IS NULL AND agent_exit = '' AND waiting != 0)",
	track.Exited.ID:         "(closed_at IS NULL AND agent_exit = 'exited')",
	track.Active.ID:         "(closed_at IS NULL AND agent_exit = '' AND waiting = 0)",
	track.Done.ID:           "(closed_at IS NOT NULL AND archived_at IS NULL)",
	track.Closed.ID:         "archived_at IS NOT NULL",
}

// FilteredTracks are the tracks f picks at now, newest first, at most
// limit.
func (s *Store) FilteredTracks(ctx context.Context, f track.Filter, now time.Time, limit int) ([]track.Track, error) {
	where := []string{"archived_at IS NULL"}
	if f.Archived {
		where[0] = "archived_at IS NOT NULL"
	}
	var args []any
	from, to := f.Range(now)
	if !from.IsZero() {
		where, args = append(where, "created_at >= ?"), append(args, from.UnixMilli())
	}
	if !to.IsZero() {
		where, args = append(where, "created_at < ?"), append(args, to.UnixMilli())
	}
	var statuses []string
	for _, id := range f.Statuses {
		if cond, ok := statusSQL[id]; ok {
			statuses = append(statuses, cond)
		}
	}
	if len(statuses) > 0 {
		where = append(where, "("+strings.Join(statuses, " OR ")+")")
	}
	query := "WHERE " + strings.Join(where, " AND ") + " ORDER BY created_at DESC, id DESC"
	// The PR statuses are matched after the query.
	if len(f.PRStatuses) == 0 {
		query, args = query+" LIMIT ?", append(args, limit)
	}
	found, err := s.tracks(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	found = slices.DeleteFunc(found, func(t track.Track) bool { return !f.Match(t, now) })
	return found[:min(len(found), limit)], nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
