-- The proxy's output ports, each forwarding to one input: a dev server
-- (input_track, input_repo, input_server) or a server found listening
-- in a track's panes (input_track, input_port). No input_track is none.
CREATE TABLE proxy_ports (
  port         INTEGER PRIMARY KEY,
  input_track  TEXT    NOT NULL DEFAULT '',
  input_repo   TEXT    NOT NULL DEFAULT '',
  input_server TEXT    NOT NULL DEFAULT '',
  input_port   INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL -- Unix milliseconds, UTC; the order they're shown in
) STRICT;
