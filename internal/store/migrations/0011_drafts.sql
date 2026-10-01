CREATE TABLE drafts (
  id        TEXT    PRIMARY KEY,
  request   TEXT    NOT NULL, -- what the form asked for, as JSON
  error     TEXT    NOT NULL, -- why the last creation from it failed
  failed_at INTEGER NOT NULL  -- when it failed first, Unix milliseconds, UTC
) STRICT;
