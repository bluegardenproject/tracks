# Draft: storage on SQLite (chunk 4)

**Status: draft.** This is worked out in design, but not yet a feature plan. Part of the [v2 masterplan](../../masterplan.md). How status is stored follows the [track status model](../../masterplan.md#track-status-to-be-designed); the `status` column below is a placeholder.

## Why

- **v1 keeps every track ever run in `state.json`.** It's rewritten in full on every status change, and the dashboard fetches all of it every second. At 41 tracks that is 142 KB per fetch, and it grows forever.
- **Finished tracks clutter the list,** and have to be removed by hand.
- **Agent hooks** ([hooks draft](hooks.md)) produce an event stream per track. That's append-heavy and needs queries, which a JSON file handles badly.

v1's dashboard sluggishness has a different cause: synchronous git calls on every cursor move (`refreshDetail`). v2 loads details asynchronously and only on request.

## What users get

- **Tracks list:**
  - **Active:** live, interrupted, waiting, or with an open PR.
  - **Recent:** the last 10 finished tracks.
  - **History:** every track, paged, with filters (repo, kind, status, provider, date, text in slug, branch or task).
- **Auto-archive:** a finished track is archived once its PR is merged or closed and its worktree is gone. "Remove" becomes "Archive"; a hard delete stays for mistakes.
- **Timeline per track:** prompts, approvals, PRs opened and merged, turns and errors, resumes and promotions, usage per turn.

## Design

- **Driver:** `modernc.org/sqlite`. It's pure Go with no cgo, so releases and cross-compiling stay simple. It adds about 6 to 8 MB to the binary.
- **File:** `<v2 data dir>/tracks.db`, in WAL mode with `busy_timeout`.
- **Only the daemon opens the database** once it exists (chunk 5). CLI and UI then go through it: one writer, rules in one place, and a change stream instead of polling. Until then the Tracks window opens it through `store` ([Repositories plan](../03-repositories.md)).
- **Connection settings, tables and backups** as in the [Repositories plan](../03-repositories.md#database-internalv2store): WAL, `busy_timeout`, `foreign_keys`, `BEGIN IMMEDIATE` writes, `STRICT` tables, and `VACUUM INTO` backups before migrating.
- **Packages:**
  - `internal/v2/store` implements the storage.
  - Domain types live in `internal/v2/track`.
  - The interface stays small: `Get`, `Update(id, reason, fn)`, `List(query)`, `AppendEvent`, `Events(id, page)`.
- **Schema sketch:**

```sql
CREATE TABLE tracks (
  id TEXT PRIMARY KEY, kind TEXT, provider TEXT, status TEXT,
  slug TEXT, branch TEXT, task_prompt TEXT, model TEXT, session_id TEXT,
  created_at INTEGER, updated_at INTEGER, finished_at INTEGER, archived_at INTEGER
);
-- repo_id links to repos; name and path are copies, so a track's history
-- survives renaming or deleting its repo. folder is the repo's folder in the
-- worktree, simplified from the name when the track starts.
CREATE TABLE track_repos (track_id TEXT, repo_id INTEGER REFERENCES repos (id) ON DELETE SET NULL,
  name TEXT, path TEXT, folder TEXT, branch TEXT, base TEXT, PRIMARY KEY (track_id, folder));
CREATE TABLE prs (track_id TEXT, url TEXT, number INTEGER, state TEXT, updated_at INTEGER,
  PRIMARY KEY (track_id, url));
CREATE TABLE events (id INTEGER PRIMARY KEY, track_id TEXT, at INTEGER,
  source TEXT, type TEXT, data TEXT);
CREATE INDEX tracks_list ON tracks (archived_at, updated_at DESC);
CREATE INDEX events_track ON events (track_id, at);
```

- **Real columns only,** no JSON catch-all: migrations are cheap, and typed columns are checked and can be indexed.
- **Migrations:** embedded SQL files, versioned by `PRAGMA user_version` and applied forward-only at open. A database newer than the binary is refused. Every change gets its own migration until the release, which folds them into one initial schema.
- **Daemon API:**
  - `list{scope: active|recent|history, filter, limit, cursor}` returns one page.
  - `events{track, limit, cursor}` returns a page of the timeline.
  - `watch` streams "track X changed" messages. The UI updates from it instead of polling.
- **Retention:** keep all events at first; add an `events_retention_days` setting if the file grows.

## Tests

- Store tests on a temp database:
  - every field round-trips
  - list scopes and filters
  - paging cursors
  - the auto-archive rule
- Migrations from empty to the latest schema, and refusal of a newer database.

## Likely PR slices

1. `store` with schema and migrations, and events for daemon-side transitions.
2. The paged `list`, auto-archive, Archive instead of Remove, `events` and `watch`.
