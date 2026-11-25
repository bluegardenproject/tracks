-- instructions' IDs are never reused: a replaced instruction's ID can't name its replacement,
-- which a stale paste would then mark typed. SQLite takes AUTOINCREMENT only at CREATE TABLE.
CREATE TABLE instructions_new (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  track_id   TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL, -- the event it answers: conflicts, ci_failed, review_threads; ask:<event> to ask the user
  pr_url     TEXT    NOT NULL, -- the PR it's about
  body       TEXT    NOT NULL,
  created_at INTEGER NOT NULL, -- Unix milliseconds, UTC
  typed_at   INTEGER           -- NULL while it waits
) STRICT;
INSERT INTO instructions_new (id, track_id, kind, pr_url, body, created_at, typed_at)
  SELECT id, track_id, kind, pr_url, body, created_at, typed_at FROM instructions;
DROP TABLE instructions;
ALTER TABLE instructions_new RENAME TO instructions;
CREATE INDEX instructions_waiting ON instructions (track_id) WHERE typed_at IS NULL;
