# Plan: archive and filters

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 4 (storage): removing tracks from Station, auto-archive and filters. The change stream came next ([12-change-stream-and-derail.md](12-change-stream-and-derail.md)); the per-track event timeline is a future feature.

## What users get

- **Archive** in an ended track's details removes the track from Station. It removes its worktrees too, as Clean does, and keeps its branches.
- **Auto-archive**, a setting under Settings → Tracks → **Tracks History**: tracks that ended more than a week ago are archived on their own, unless one of their PRs is still open.
- **Tracks filter** in Quick Access: show only some statuses, only archived tracks, or only tracks started between two dates. Station says when a filter is on, and clears it with one key.
- **Unarchive** in an archived track's details puts it back in Station.

## Archive

- **Only ended tracks,** done or closed. An open track shows End, as now; Archive appears once it's ended.
- **Archive removes the worktrees** of a done track first, with Clean's checks: without unsaved work it asks "Archive rate-bug? Its worktrees are removed; its branches stay." With unsaved work it lists it, as Clean does, and offers "Remove and archive" or Cancel. A closed track is archived at once.
- **Unarchive** puts the track back in Station, as it was: closed, or done when its worktrees were kept (see auto-archive). Unarchive is shown in the details of an archived track, which Station shows under the "Archived" filter.
- **Keys:** `a` archive, `u` unarchive.

## Auto-archive

- **Settings → Tracks** gets a **Tracks History** group below the track types:
  - **Auto-archive tracks** (checkbox, off by default): archive tracks that ended more than 7 days ago and have no PR still open or in draft.
  - **Tracks with unsaved work** (shown while auto-archive is on): **Skip them** (the default; they stay in Station until archived by hand) or **Archive, keep the worktrees**.
- **The daemon** runs it when it starts and every hour, and logs what it archived.
- **Saved** in `settings.yaml`:

```yaml
history:
  auto_archive: true
  unsaved: skip # or keep
```

## Tracks filter

- **Quick Access** gets **Tracks filter** (`f`). It opens a popup:
  - **Status:** checkboxes for the track statuses (active, action required, done, closed) and the PR statuses (no PR, PR open, PR merged, PR closed). None ticked in a group means all of it.
  - **Archived only:** shows the archived tracks instead of the others.
  - **Started:** Any time, Today, Last 7 days, Last 30 days, or Between, with From and To as `YYYY-MM-DD`, both included, in local time. Either date may be left empty. Last 7 days is today and the six days before, from local midnight; Last 30 days likewise.
  - **Apply**, **Clear** and **Cancel**. Apply and Clear switch to the Tracks window.
- **Kept until cleared,** also across restarts: the daemon stores it, and Station's `list` returns the filtered tracks and the filter.
- **Station** shows a line above the table while a filter is on, such as **Filtered: done, closed · PR merged · started in the last 7 days**, with **Clear filter** (`x`).
- **Archived tracks** show Unarchive (`u`, also Enter) instead of Resume, Clean and Archive.
- **Unfiltered,** Station lists as now: the open tracks, then the last 100 ended ones, without the archived. **Filtered,** it searches every track, newest first, up to 500.
- Later: a text search on the name, branch and prompt; filters by repo, type and engine.

## Other changes

- **GitHub poll every minute** instead of every 2 minutes.
- **The footer's system data** every 15 seconds instead of 5 (tmux's `status-interval`).

## Storage

- `0006_archive.sql`: `tracks.archived_at`, and the index Station's list uses.
- `0007_filter.sql` (with the filter): `station_filter`, one row with real columns: the track statuses and PR statuses picked, archived only, from and to.

## Model

- `track.State` gains `ArchivedAt`. Two events: `archived` (only on an ended track) and `unarchived`. Status stays done or closed; archived is shown by where the track is listed, not as a status.
- `tracks`: `Archive(id, check, force)` (Clean's steps, then `archived`, or `archived` alone to keep the worktrees), `Unarchive(id)`, `AutoArchive(now)`, and `List(filter)`.
- `rpc`, `daemon`: `archive`, `unarchive`, `filter` (get and set); `list` applies the stored filter; the auto-archive timer.

## Packages

- **New: `ui/tracksfilter`:** the filter popup, beside `ui/addtrack` and `ui/quickaccess`. The filter's own type and its matching live in `track`, so the store, the daemon and the UI share them.
- `settings`: the `history` block. `ui/tracksview`: Archive, Unarchive, the Tracks History group and the "Filtered" line. `ui/quickaccess`, `cli`: the new entry and its popup.

Sync before building: the new `ui/tracksfilter` package.

## Tests

- **`track`:** `archived` and `unarchived` from every status; the filter matching each status group, archived only, and the date edges (From and To included, local midnight).
- **`store`:** the migrations; archived tracks out of the unfiltered list; the filter's query and its row saved and read back.
- **`tracks`:** Archive with and without unsaved work, and on a closed track; Unarchive; AutoArchive at 7 days, skipping open PRs, with both unsaved-work settings.
- **`ui`:** the Archive and Unarchive buttons and their questions; the Tracks History group saving; the filter popup (ticks, presets, custom dates rejected when invalid or From after To); Station's "Filtered" line and Clear.
- **By hand:** archive and unarchive a track; auto-archive with a track back-dated in an isolated database; each filter from Quick Access.

## Delivery

Two branches, each its own PR:
1. **`feat/v2-archive`:** this plan; `archived_at`; Archive and auto-archive with the Tracks History settings; the 1-minute GitHub poll and the 15-second footer. Playable: archive by hand, and turn auto-archive on.
2. **`feat/v2-filters`:** the filter, stored in the daemon; the Quick Access entry and its popup; Station's "Filtered" line; Unarchive; docs and the masterplan.
