-- A track's block of ports for its dev servers, claimed by its first
-- server. An ended track's block goes to the next track that needs one.
CREATE TABLE track_ports (
  track_id TEXT    PRIMARY KEY REFERENCES tracks (id) ON DELETE CASCADE,
  base     INTEGER NOT NULL UNIQUE -- the block's first port
) STRICT;
