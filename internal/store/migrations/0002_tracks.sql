CREATE TABLE tracks (
  id          TEXT    PRIMARY KEY, -- 20260928-151500-a1b2c3
  kind        TEXT    NOT NULL CHECK (kind IN ('work', 'ask', 'plan', 'review', 'doc')),
  name        TEXT    NOT NULL,    -- the window's name
  engine      TEXT    NOT NULL,    -- claude, cursor
  model       TEXT    NOT NULL,    -- '' is the engine's own default
  session_id  TEXT    NOT NULL,
  prompt      TEXT    NOT NULL,    -- as the user wrote it, without Tracks' additions
  review_ref  TEXT    NOT NULL DEFAULT '',
  document    TEXT    NOT NULL DEFAULT '',
  candor      INTEGER NOT NULL DEFAULT 0,
  opinion     INTEGER NOT NULL DEFAULT 1 CHECK (opinion IN (0, 1)),
  claim_check INTEGER NOT NULL DEFAULT 1 CHECK (claim_check IN (0, 1)),
  terminal    INTEGER NOT NULL DEFAULT 0 CHECK (terminal IN (0, 1)),
  created_at  INTEGER NOT NULL,    -- Unix milliseconds, UTC
  closed_at   INTEGER              -- the window closed
) STRICT;
CREATE INDEX tracks_open ON tracks (closed_at, created_at);

-- name, path and base are copies, so a track's history survives
-- renaming or removing its repo.
CREATE TABLE track_repos (
  track_id TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  position INTEGER NOT NULL,       -- the order picked; 0 is where the agent starts
  repo_id  INTEGER REFERENCES repos (id) ON DELETE SET NULL,
  name     TEXT    NOT NULL,
  path     TEXT    NOT NULL,       -- the primary checkout
  worktree TEXT    NOT NULL DEFAULT '',
  branch   TEXT    NOT NULL DEFAULT '',
  base     TEXT    NOT NULL,
  PRIMARY KEY (track_id, position)
) STRICT;
CREATE INDEX track_repos_repo ON track_repos (repo_id);
