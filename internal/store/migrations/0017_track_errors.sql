-- A track's server and setup errors, from their panes, until cleared.
CREATE TABLE track_errors (
  track_id TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  kind     TEXT    NOT NULL CHECK (kind IN ('server', 'setup')),
  subject  TEXT    NOT NULL, -- repo/server for a server, the repo for a setup
  code     INTEGER NOT NULL, -- the exit code
  at       INTEGER NOT NULL, -- Unix milliseconds, UTC
  PRIMARY KEY (track_id, kind, subject)
) STRICT;
