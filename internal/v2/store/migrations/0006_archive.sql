ALTER TABLE tracks ADD COLUMN archived_at INTEGER; -- taken out of Station
CREATE INDEX tracks_ended ON tracks (archived_at, closed_at);
