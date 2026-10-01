ALTER TABLE tracks ADD COLUMN waiting INTEGER NOT NULL DEFAULT 0 CHECK (waiting IN (0, 1)); -- its agent waits on a dialog
