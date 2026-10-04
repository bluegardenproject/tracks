ALTER TABLE repos ADD COLUMN setup TEXT NOT NULL DEFAULT ''; -- run once in a worktree before its dev servers; '' for none

CREATE TABLE dev_servers (
  repo_id   INTEGER NOT NULL REFERENCES repos (id) ON DELETE CASCADE,
  position  INTEGER NOT NULL, -- the order in the repo form
  name      TEXT    NOT NULL COLLATE NOCASE,
  command   TEXT    NOT NULL,
  dir       TEXT    NOT NULL, -- relative to the repo; '' for its root
  port_mode TEXT    NOT NULL CHECK (port_mode IN ('assigned', 'fixed', 'detect')),
  port      INTEGER NOT NULL, -- the fixed port; 0 for the other modes
  type      TEXT    NOT NULL, -- a label such as rspack; '' for none
  PRIMARY KEY (repo_id, position),
  UNIQUE (repo_id, name)
) STRICT;
