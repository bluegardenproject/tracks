-- The envelope's first fields (docs/design/protocol.md): the outcome a
-- sender gives its message, '' for none, and what it refers to, its PRs'
-- URLs and files' paths as JSON, '' for nothing. Values are checked in
-- internal/relay, so the set of outcomes can grow without a migration.
ALTER TABLE relay_messages ADD COLUMN outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE relay_messages ADD COLUMN refs TEXT NOT NULL DEFAULT '';
