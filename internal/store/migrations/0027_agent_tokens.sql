-- The tokens of the agent sessions Tracks starts: tracks mcp shows the
-- daemon its agent's token, and the daemon knows the agent by it
-- (docs/design/protocol.md). Only a token's hash is kept.
CREATE TABLE agent_tokens (
  hash     TEXT    PRIMARY KEY, -- the token's SHA-256, hex
  track_id TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  holder   TEXT    NOT NULL DEFAULT '', -- '' for the track's own agent; later a train's handle
  at       INTEGER NOT NULL  -- Unix milliseconds, UTC
) STRICT;

CREATE UNIQUE INDEX agent_tokens_holder ON agent_tokens (track_id, holder);
