# Plan: failed creations kept as drafts

**Status: planned.** Part of the [v2 masterplan](../masterplan.md), the [v2.0.0 scope](../masterplan.md#v200-scope). A creation that fails today loses what was typed once the form is closed: the error shows in the status line, or on the form while it's open. This plan keeps it as a draft in Station, to start again.

## What users get

- **Every failed creation becomes a draft:** the track type, name, repos, prompt, options, agent and model, and why it failed. It doesn't matter whether the form was still open.
- **A draft is a row in Station** with the **draft** status (warning badge), under the name typed or else the prompt's first line. Its details show why it failed and what was typed.
- **Start again** (Enter, or its button) opens the New track form filled in with the draft, to change and create. **Discard** (`x`) deletes it.
- **A draft goes away when a creation from it succeeds:** from the form it opened, or from the form that failed, when you retry there. Another failure updates the draft's reason instead of adding one.
- **Station's filter:** drafts show when no filter is on, and under a filter only when its statuses include draft.

## How it works

- **Store:** a `drafts` table (migration `0011_drafts.sql`): `id`, the request as JSON, the error, and when it failed first. `SaveDraft` inserts or updates by ID; `Drafts`, `Draft` and `DeleteDraft`. Writes go through `Watched`, so Station hears them.
- **`tracks.Request.Draft`** is the draft's ID. The form picks one the first time it creates and keeps it for its retries; Start again passes the draft's. A creation without one, such as from the CLI, gets a new ID.
- **`Create`** saves the draft when it fails, with the error as the form shows it, and deletes the draft when it succeeds.
- **`Station`** adds the drafts as `Listed` rows with a `Draft` part (the error, the prompt), whose status is `track.Drafted`. It's declared with the other statuses; no stored track has it.
- **RPC:** `draft` returns a draft's request, `discard-draft` deletes one. The form's popup takes `--draft <id>` and fills itself from the request; a repo or document that no longer exists shows as the form's usual field errors.

## Packages

- `track`: the draft status.
- `store`: the table and its queries.
- `tracks`: `Request.Draft`, drafts from `Create`, `Station`, `Draft`, `DiscardDraft`.
- `rpc`, `daemon`, `cli`: the two calls; `popup add-track --draft`.
- `ui/addtrack`: filling the form from a request; picking the draft ID.
- `ui/source`, `ui/tracksview`: the draft row, its details and actions.

## Not in this plan

- Keeping a form closed without creating: that's what its "Discard this track?" question is for.
- Drafts in the footer: a draft has no window.

## Tests

- `store`: saving, updating, listing and deleting drafts; the migration.
- `tracks`: a failed creation keeps a draft, a retry with the same ID updates it, a success deletes it; Station lists drafts without a filter, and under one only for the draft status.
- `ui/addtrack`: a form filled from a request builds the same request back, for each track type; the draft ID stays across retries.
- `ui/tracksview`: a draft row shows its status and reason; Start again and Discard call their functions.
- **By hand:** a creation failing with the form open and with it closed; starting a draft again; discarding one.
