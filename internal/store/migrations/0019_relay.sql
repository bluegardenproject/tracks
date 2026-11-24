-- The relay: messages between a track and its trains (docs/design/trains.md).
CREATE TABLE relay_messages (
  id        INTEGER PRIMARY KEY,
  track_id  TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  sender    TEXT    NOT NULL, -- a handle: 'track', a train's, or 'tracks' for a notice
  recipient TEXT    NOT NULL, -- a handle; '' for a notice to all
  kind      TEXT    NOT NULL CHECK (kind IN ('report', 'event', 'question', 'answer', 'notice')),
  body      TEXT    NOT NULL,
  at        INTEGER NOT NULL  -- Unix milliseconds, UTC
) STRICT;

CREATE INDEX relay_messages_track ON relay_messages (track_id, id);

-- How far each reader of a track's relay has read: the last message's id.
CREATE TABLE relay_cursors (
  track_id TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  reader   TEXT    NOT NULL, -- a handle
  last_id  INTEGER NOT NULL,
  PRIMARY KEY (track_id, reader)
) STRICT;
