package store

import (
	"context"
	"database/sql"
)

// Port modes of a dev server: how the port it serves on is known.
const (
	PortAssigned = "assigned" // Tracks picks one and passes it as $PORT
	PortFixed    = "fixed"    // the server always uses Port
	PortDetect   = "detect"   // read from what the server listens on
)

// DevServer is a named command of a repo that serves on a port.
type DevServer struct {
	Name     string
	Command  string
	Dir      string // relative to the repo; "" for its root
	PortMode string
	Port     int    // for PortFixed; 0 otherwise
	Type     string // a label such as rspack; "" for none
}

// devServers returns the dev servers matching where, by repo and in
// their order.
func (s *Store) devServers(ctx context.Context, where string, args ...any) (map[int64][]DevServer, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT repo_id, name, command, dir, port_mode, port, type FROM dev_servers "+where+" ORDER BY repo_id, position", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]DevServer{}
	for rows.Next() {
		var id int64
		var d DevServer
		if err := rows.Scan(&id, &d.Name, &d.Command, &d.Dir, &d.PortMode, &d.Port, &d.Type); err != nil {
			return nil, err
		}
		out[id] = append(out[id], d)
	}
	return out, rows.Err()
}

// putDevServers replaces the repo's dev servers with servers.
func putDevServers(ctx context.Context, tx *sql.Tx, repo int64, servers []DevServer) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM dev_servers WHERE repo_id = ?", repo); err != nil {
		return err
	}
	for i, d := range servers {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO dev_servers (repo_id, position, name, command, dir, port_mode, port, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			repo, i, d.Name, d.Command, d.Dir, d.PortMode, d.Port, d.Type); err != nil {
			return err
		}
	}
	return nil
}
