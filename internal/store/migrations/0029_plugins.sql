-- The installed plugins.
CREATE TABLE plugins (
  id           INTEGER PRIMARY KEY,
  name         TEXT    NOT NULL UNIQUE,      -- the manifest's name
  source       TEXT    NOT NULL,             -- a git URL, or a local folder's absolute path
  local        INTEGER NOT NULL DEFAULT 0,   -- a local folder: linked, never pinned
  ref          TEXT    NOT NULL DEFAULT '',  -- the branch, tag or commit asked for; '' for the remote's default
  pinned       TEXT    NOT NULL DEFAULT '',  -- the commit installed; '' for a local plugin
  dir          TEXT    NOT NULL,             -- where it's installed
  enabled      INTEGER NOT NULL DEFAULT 1,
  approved     TEXT    NOT NULL,             -- what the user approved, as JSON
  installed_at INTEGER NOT NULL,             -- Unix milliseconds, UTC
  updated_at   INTEGER NOT NULL
) STRICT;
