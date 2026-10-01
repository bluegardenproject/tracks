ALTER TABLE tracks ADD COLUMN interrupted INTEGER NOT NULL DEFAULT 0 CHECK (interrupted IN (0, 1)); -- its window closed with Tracks, to reopen
