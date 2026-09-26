# Plan: Repositories tab and the database

**Status: built.** Part of the [v2 masterplan](../masterplan.md). This is the first real feature: repos live in SQLite and are managed on the Repositories tab of the Tracks window. It starts chunk 4 ([storage draft](drafts/storage.md)) with the repos table only.

## What users get

- **List** on the left quarter, unframed for now: a **New** button at the top, then the repos (name, base branch).
- **Details** on the other three quarters, in a frame, editable. Each field's title sits above its input (on `input.bg`):
  - name; path to the primary checkout, with `origin`'s web address under it when there is one (read-only, credentials removed); base branch, `main` when left empty
  - an **Options** section: "open PRs as drafts"
  - an **Active tracks** section for saved repos: how many run in it and their names, or "None"
  - **Save** appears once something changed; **Delete** is always at the bottom and asks first
- **New:** the details show an empty form, name first. Once the path is filled in, Tracks checks it's a git checkout and fills in whatever is still empty: the name (the folder's) and the base branch (what `origin/HEAD` points to).
- **Focus:** Enter or a click on the details moves focus into the form; Tab and Shift+Tab move between its fields and buttons; Esc goes back to the list. The form's frame uses `border.accent` while it has focus.
- **Unsaved changes:** selecting another repo, Esc or switching tabs asks: Save, Discard or Cancel.
- Works in both `--new-app` and `--new-app --demo`: the database is always the real one. Demo tracks stay fake tmux windows and never reach the database.

## Rules (`internal/v2/repos`)

- **Path:** absolute, cleaned, symlinks resolved, and the top level of a git checkout (`git rev-parse --show-toplevel`). `~/` is expanded. One repo per path.
- **Name:** what Tracks shows; any printable text up to 64 characters, spaces included. Unique regardless of case (SQLite's `NOCASE`, which folds only A–Z), so two repos never look alike in the list.
  - Tracks refer to a repo by its `id`, never by name, so renaming is a plain update.
  - When a track starts, the repo's folder in the track's worktree gets a simplified name (`Shop Web` → `shop-web`, `-2` on a clash within the track), copied onto the track so a later rename never moves a worktree.
- **Base branch:** must exist locally or on `origin`; empty means `main`.
- **Active tracks:** until tracks are in the database, a track uses a repo when its window's `@tracks_repo` names it.
  - Name and path can't change while active tracks use the repo: their worktrees belong to it. Base branch and drafts can.
  - Delete is refused while active tracks use it. Later, finished tracks keep a copy of the repo's name and path, so deleting doesn't break their history.

## Database (`internal/v2/store`)

- **File:** `tracks.db` in the v2 data folder (`~/.local/state/tracks-v2/`; the folder loses `-v2` at the release). Always local: SQLite's locking breaks in iCloud or Dropbox folders.
- **Driver:** `modernc.org/sqlite`, pure Go, so `CGO_ENABLED=0` builds keep working. Adds about 6 to 8 MB; only v2 builds include it.
- **Connection settings:**
  - WAL mode, `busy_timeout` 5 s, `synchronous=NORMAL` (a crash loses nothing; a power cut at most the last transaction).
  - `foreign_keys=ON` on every connection, since SQLite leaves it off per connection.
  - Write transactions take the write lock up front (`BEGIN IMMEDIATE`), so two writers wait instead of failing with "database is busy".
  - One connection for now; the daemon can split readers and a writer later.
- **Tables are `STRICT`,** so a wrong type is an error instead of silently stored.
- **Migrations:** numbered SQL files embedded in the binary, applied forward-only at open, tracked in `PRAGMA user_version`. A database newer than the binary is refused. Before migrating an existing database, `VACUUM INTO` writes a consistent backup (a plain file copy can miss WAL content); the last three are kept.
- **Before the release** every schema change still gets its own migration, so nobody loses their repos; the migrations are folded into one initial schema right before v2.0.0.
- **Who opens it:** for now the Tracks window, through `store`. The daemon takes it over in chunk 5 behind the same `ui/source` interface.

```sql
CREATE TABLE repos (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  path        TEXT    NOT NULL UNIQUE,
  base_branch TEXT    NOT NULL,
  draft_prs   INTEGER NOT NULL DEFAULT 0 CHECK (draft_prs IN (0, 1)),
  created_at  INTEGER NOT NULL,  -- Unix milliseconds, UTC
  updated_at  INTEGER NOT NULL
) STRICT;
```

Tracks will refer to repos by `id`, so renaming a repo is a plain update.

## Packages

- `store`: opens the database, migrations, and queries (`Repos`, `AddRepo`, `UpdateRepo`, `DeleteRepo`). Maps unique-constraint failures to `ErrNameTaken` and `ErrPathTaken`.
- `repos`: the rules above over `store` and git. Git calls sit behind a small interface, tested once against a real throwaway repo.
- `ui/source`: gains a `Repos` interface for the tab; `tracksview` gets a `repositories` screen next to `station`.

## Tests

- `store`: every field round-trips, unique name (any case) and path, migrations from empty, refusing a newer database, the backup before a migration.
- `repos`: path, name and base branch checks against a throwaway git repo; renaming and deleting refused while tracks use the repo.
- `tracksview`: New, editing shows Save, Delete asks first.

## Delivery

One PR, since only the tab makes it playable, in commits: `store`, then `repos`, then the tab.
