ALTER TABLE tracks ADD COLUMN parent_id TEXT REFERENCES tracks (id) ON DELETE SET NULL; -- the track `tracks new` ran in; NULL for none, or once it's deleted
CREATE INDEX tracks_parent ON tracks (parent_id) WHERE parent_id IS NOT NULL;
