-- Prompts Tracks types into a track's agent once it's free, as a side track's GitHub policies say.
CREATE TABLE instructions (
  id         INTEGER PRIMARY KEY,
  track_id   TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL, -- the event it answers: conflicts, ci_failed, review_threads, ask
  pr_url     TEXT    NOT NULL, -- the PR it's about
  body       TEXT    NOT NULL,
  created_at INTEGER NOT NULL, -- Unix milliseconds, UTC
  typed_at   INTEGER           -- NULL while it waits
) STRICT;
CREATE INDEX instructions_waiting ON instructions (track_id) WHERE typed_at IS NULL;
