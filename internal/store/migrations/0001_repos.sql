CREATE TABLE repos (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  path        TEXT    NOT NULL UNIQUE,
  base_branch TEXT    NOT NULL,
  draft_prs   INTEGER NOT NULL DEFAULT 0 CHECK (draft_prs IN (0, 1)),
  created_at  INTEGER NOT NULL, -- Unix milliseconds, UTC
  updated_at  INTEGER NOT NULL
) STRICT;
