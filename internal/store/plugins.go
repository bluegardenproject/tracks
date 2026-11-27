package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Plugin is an installed plugin.
type Plugin struct {
	ID     int64
	Name   string
	Source string // a git URL, or a local folder's absolute path
	Local  bool   // linked from Source, never pinned
	Ref    string // the branch, tag or commit asked for; "" for the remote's default
	Pinned string // the commit installed; "" for a local plugin
	Dir    string // where it's installed
	// Enabled plugins are offered to tracks; disabled ones stay
	// installed.
	Enabled bool
	// Approved is what the user approved, as JSON.
	Approved    string
	InstalledAt time.Time
	UpdatedAt   time.Time
}

const pluginColumns = "id, name, source, local, ref, pinned, dir, enabled, approved, installed_at, updated_at"

// Plugins lists the installed plugins by name.
func (s *Store) Plugins(ctx context.Context) ([]Plugin, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+pluginColumns+" FROM plugins ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Plugin returns the plugin called name.
func (s *Store) Plugin(ctx context.Context, name string) (Plugin, error) {
	p, err := scanPlugin(s.db.QueryRowContext(ctx, "SELECT "+pluginColumns+" FROM plugins WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return Plugin{}, ErrNotFound
	}
	return p, err
}

// AddPlugin stores p as a new plugin; ErrNameTaken when one has its
// name.
func (s *Store) AddPlugin(ctx context.Context, p Plugin) (Plugin, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO plugins (name, source, local, ref, pinned, dir, enabled, approved, installed_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		p.Name, p.Source, p.Local, p.Ref, p.Pinned, p.Dir, p.Enabled, p.Approved, now, now)
	if err != nil {
		return Plugin{}, constraint(err)
	}
	return s.Plugin(ctx, p.Name)
}

// UpdatePlugin stores what an update changes: the commit, the folder
// and what was approved.
func (s *Store) UpdatePlugin(ctx context.Context, p Plugin) (Plugin, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE plugins SET pinned = ?, dir = ?, approved = ?, updated_at = ? WHERE name = ?",
		p.Pinned, p.Dir, p.Approved, time.Now().UnixMilli(), p.Name)
	if err := affected(res, err); err != nil {
		return Plugin{}, err
	}
	return s.Plugin(ctx, p.Name)
}

// SetPluginEnabled enables or disables the plugin called name.
func (s *Store) SetPluginEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, "UPDATE plugins SET enabled = ?, updated_at = ? WHERE name = ?",
		enabled, time.Now().UnixMilli(), name)
	return affected(res, err)
}

// DeletePlugin removes the plugin called name.
func (s *Store) DeletePlugin(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM plugins WHERE name = ?", name)
	return affected(res, err)
}

// affected turns an update that matched no row into ErrNotFound.
func affected(res sql.Result, err error) error {
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

func scanPlugin(row scanner) (Plugin, error) {
	var p Plugin
	var installed, updated int64
	err := row.Scan(&p.ID, &p.Name, &p.Source, &p.Local, &p.Ref, &p.Pinned, &p.Dir, &p.Enabled, &p.Approved, &installed, &updated)
	p.InstalledAt, p.UpdatedAt = time.UnixMilli(installed), time.UnixMilli(updated)
	return p, err
}
