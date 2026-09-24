# Draft: storage on SQLite (chunk 4)

**Status: draft.** This is worked out in design, but not yet a feature plan. Part of the [v2 masterplan](../../masterplan.md). How status is stored follows the [track status model](../../masterplan.md#track-status-to-be-designed-before-chunk-2); the `status` column below is a placeholder.

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
- **Only the daemon opens the database.** CLI and UI go through the daemon, so there's no locking between processes.
- **Packages:**
  - `internal/v2/store` implements the storage.
  - Domain types live in `internal/v2/track`.
  - The interface stays small: `Get`, `Update(id, reason, fn)`, `List(query)`, `AppendEvent`, `Events(id, page)`.
- **Schema sketch:**

```sql
CREATE TABLE tracks (
  id TEXT PRIMARY KEY, kind TEXT, provider TEXT, status TEXT,
  slug TEXT, branch TEXT, task_prompt TEXT, model TEXT, session_id TEXT,
  created_at INTEGER, updated_at INTEGER, finished_at INTEGER, archived_at INTEGER,
  extra TEXT -- JSON for rarely queried fields, so adding one needs no migration
);
CREATE TABLE track_repos (track_id TEXT, name TEXT, path TEXT, branch TEXT, base TEXT,
  PRIMARY KEY (track_id, name));
CREATE TABLE prs (track_id TEXT, url TEXT, number INTEGER, state TEXT, updated_at INTEGER,
  PRIMARY KEY (track_id, url));
CREATE TABLE events (id INTEGER PRIMARY KEY, track_id TEXT, at INTEGER,
  source TEXT, type TEXT, data TEXT);
CREATE INDEX tracks_list ON tracks (archived_at, updated_at DESC);
CREATE INDEX events_track ON events (track_id, at);
```

- **Columns vs `extra`:** anything the list filters or sorts on is a column; the rest lives in `extra`.
- **Migrations:** embedded SQL files, versioned by `PRAGMA user_version` and applied forward-only at daemon start. The file is copied before each migration. A database newer than the binary is refused.
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
