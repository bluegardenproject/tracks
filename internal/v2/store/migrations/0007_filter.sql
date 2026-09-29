-- Station's filter: one row while a filter is on. The statuses are
-- comma-separated IDs; from_date and to_date are YYYY-MM-DD.
CREATE TABLE station_filter (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    statuses TEXT NOT NULL DEFAULT '',
    pr_statuses TEXT NOT NULL DEFAULT '',
    archived INTEGER NOT NULL DEFAULT 0,
    started TEXT NOT NULL DEFAULT '',
    from_date TEXT NOT NULL DEFAULT '',
    to_date TEXT NOT NULL DEFAULT ''
);
CREATE INDEX tracks_created ON tracks (created_at);
