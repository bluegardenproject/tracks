ALTER TABLE repos ADD COLUMN prefix TEXT NOT NULL DEFAULT ''; -- put before the name a track in this repo, and at most one other, is shown under; '' for none
