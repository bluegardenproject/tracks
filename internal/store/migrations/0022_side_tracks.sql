-- A side track's handle on its main track's relay (parent_id); NULL for every other track.
ALTER TABLE tracks ADD COLUMN side TEXT;
CREATE UNIQUE INDEX tracks_side ON tracks (parent_id, side) WHERE side IS NOT NULL;
