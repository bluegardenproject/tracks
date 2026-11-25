-- What the poll last read of a PR, so @github's events go once per change, across restarts.
ALTER TABLE track_prs ADD COLUMN mergeable TEXT CHECK (mergeable IN ('clean', 'conflicts')); -- NULL until GitHub computed it
ALTER TABLE track_prs ADD COLUMN checks TEXT CHECK (checks IN ('pending', 'passing', 'failing')); -- NULL for none
ALTER TABLE track_prs ADD COLUMN head TEXT;       -- the commit the checks ran on
ALTER TABLE track_prs ADD COLUMN threads INTEGER; -- open review threads, last counted; NULL until counted
