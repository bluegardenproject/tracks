-- A track's pull requests: reported by its agent's hooks, or found by
-- the poll on its branches.
CREATE TABLE track_prs (
  track_id   TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  url        TEXT    NOT NULL, -- https://github.com/<owner>/<name>/pull/<number>
  repo       TEXT    NOT NULL, -- <owner>/<name>
  number     INTEGER NOT NULL,
  state      TEXT    NOT NULL CHECK (state IN ('open', 'draft', 'merged', 'closed')),
  found_at   INTEGER NOT NULL, -- Unix milliseconds, UTC
  checked_at INTEGER,          -- the poll last asked GitHub
  PRIMARY KEY (track_id, url)
) STRICT;
CREATE INDEX track_prs_state ON track_prs (state);
