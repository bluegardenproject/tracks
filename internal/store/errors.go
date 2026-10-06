package store

import (
	"context"
	"errors"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsForeignKey reports whether err is a write naming a row, such as a
// track, that isn't there.
func IsForeignKey(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

// AddTrackError records e on track id, replacing an error of the same
// kind and subject.
func (s *Store) AddTrackError(ctx context.Context, id string, e track.Failure) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO track_errors (track_id, kind, subject, code, at) VALUES (?, ?, ?, ?, ?) "+
			"ON CONFLICT (track_id, kind, subject) DO UPDATE SET code = excluded.code, at = excluded.at",
		id, e.Kind, e.Subject, e.Code, time.Now().UnixMilli())
	return err
}

// ClearTrackErrors removes track id's errors of kind about subject; ""
// for either is any. It reports whether there were some.
func (s *Store) ClearTrackErrors(ctx context.Context, id, kind, subject string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM track_errors WHERE track_id = ? AND (? = '' OR kind = ?) AND (? = '' OR subject = ?)",
		id, kind, kind, subject, subject)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) addErrors(ctx context.Context, tracks []track.Track, index map[string]int) error {
	ids, marks := idList(tracks)
	rows, err := s.db.QueryContext(ctx, "SELECT track_id, kind, subject, code FROM track_errors "+
		"WHERE track_id IN ("+marks+") ORDER BY track_id, at, kind, subject", ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var e track.Failure
		if err := rows.Scan(&id, &e.Kind, &e.Subject, &e.Code); err != nil {
			return err
		}
		i := index[id]
		tracks[i].Failures = append(tracks[i].Failures, e)
	}
	return rows.Err()
}
