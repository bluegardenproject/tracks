ALTER TABLE track_repos ADD COLUMN setup_done INTEGER NOT NULL DEFAULT 0 CHECK (setup_done IN (0, 1)); -- the repo's setup succeeded in this worktree
