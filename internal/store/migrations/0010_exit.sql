ALTER TABLE tracks ADD COLUMN agent_exit TEXT NOT NULL DEFAULT '' CHECK (agent_exit IN ('', 'exited', 'failed')); -- how its agent exited, '' while it runs
