# Plan: track status

**Status: planned.** Part of the [v2 masterplan](../masterplan.md): the [track status model](../masterplan.md#track-status), and the status part of chunk 6 (agent hooks). A track has two statuses, what it's doing and where its pull requests are, and anything can change them: the Tracks window's actions, the agent's hooks, and a GitHub poll.

## What users get

- **Station's Status column and the details** show both statuses, for example **active · PR open**, **action required**, **done · PRs merged**.
- **Action required** appears the moment the agent opens a dialog in its window: a permission prompt, a question, a plan to approve. It goes back to **active** as soon as you answer. A finished turn stays **active**.
- **The footer's track slot** gets a badge while its track needs you, on every window.
- **PRs are found on their own:** the agent's hooks report a PR right after it's opened, and every 2 minutes Tracks asks GitHub about the track's PRs and branches, so a PR opened by hand shows up too, and merges show without the agent.

## The statuses

Declared once, in `track/status.go`. Each value has an ID (stored), a label, a colour token, a priority and whether it needs attention. Station, the details and the footer only read from there.

**Track status**, one of:

| ID | Label | When | Attention |
|---|---|---|---|
| `active` | active | the track's window is open (the default) | no |
| `action_required` | action required | the agent waits on a dialog in its window | yes |
| `done` | done | ended: its window is closed | no |
| `closed` | closed | cleaned: its worktrees are gone | no |

**PR status**, derived from the track's PRs, one of:

| ID | Label | When |
|---|---|---|
| `none` | (nothing shown) | no PRs |
| `open` | PR open / 2 PRs open | at least one is open; a draft counts as open |
| `merged` | PR merged / PRs merged | none is open, and at least one was merged |
| `closed` | PR closed / PRs closed | all were closed without merging |

- **Priority** orders what's shown first where there's room for one: action required, then PR open, then the rest.
- **Adding a value** is an entry in `status.go`, the event that sets it, and their tests. A third group (for example CI checks) is a new group there, with the same shape.

## How a status changes

**One way in:** everything reports an event to the daemon, and one pure function decides the new status: `track.Apply(Status, Event) Status`. The daemon stores the result and sets the footer's option. Nothing else writes a status.

| Event | From | Effect |
|---|---|---|
| `created`, `resumed` | Create, Resume | active; the agent's dialog state cleared |
| `ended` | End, or the sweep finding the window gone | done |
| `cleaned` | Clean | closed |
| `agent.waiting` | a hook, or the pane check | action required, while active |
| `agent.working` | a hook, or the pane check | active, while action required |
| `pr.seen` (URL) | a hook, or the poll finding a PR on a branch | adds the PR as open, if it's new |
| `pr.polled` (URL, state) | the poll | updates that PR |

- Done and closed stay as they are: the lifecycle columns (`closed_at`, `cleaned_at`) already record them, and the track status is derived from them plus the agent's dialog state. So **agent events on an ended track are dropped**, and PR events are always taken.
- **The daemon method `report`** takes `{TrackID, Event, URL, State}`. `tracks hook` (below) calls it; the daemon's own actions call the same function in-process.

## Agent hooks

- **Installed per track, never in global files:** Claude gets a per-track settings file with `--settings <path>`, and Cursor a per-track plugin with `--plugin-dir <path>`, both under the track's data directory. They're written at Create and Resume, and removed at Clean. `~/.claude` and `~/.cursor` are never touched.
- **The command** is `'<abs path>/tracks' hook --engine <claude|cursor> --track <id> <event>`, with a 5-second timeout. The track ID is baked in, so it doesn't depend on the environment.
- **`tracks hook`** (hidden) reads stdin (capped at 1 MiB), keeps only the fields it maps, and sends one `report` with a 1-second timeout. It never affects the agent: it always exits 0, prints nothing for Claude and `{}` for Cursor, and logs failures to `<data>/logs/hook.log`.

**Claude:**
- **Waiting:** `PermissionRequest` (fires the moment a permission dialog opens); `PreToolUse` for `AskUserQuestion` and `ExitPlanMode`; `Elicitation`. `Notification` with `permission_prompt` or `elicitation_dialog` is the backstop, about 6 seconds late.
- **Working:** `UserPromptSubmit`, `PostToolUse`, `PostToolUseFailure`, `PermissionDenied`, `ElicitationResult`, `Stop`, `StopFailure`.
- **PRs:** `PostToolUse` for `Bash` running `gh pr create` reports the PR URL in its output, and `Stop` reports `TRACKS_PR_URL=` lines in `last_assistant_message`.

**Cursor:**
- **Working:** `beforeSubmitPrompt`, `postToolUse`, `postToolUseFailure`, `stop`.
- **Waiting:** Cursor has no hook that fires when a dialog opens, so the pane check below finds it.
- **PRs:** `afterShellExecution` for `gh pr create`, and `TRACKS_PR_URL=` lines in `afterAgentResponse`.
- **Not subscribed:** `preToolUse` and the `before*Execution` hooks. They are permission hooks: a wrong answer blocks the action, and `allow` would skip the user's approval.

## The pane check

The daemon's 2-second tick already sweeps the windows. It also reads the agent's pane (`capture-pane`, the last screen only) in two cases:
- **A Claude track in action required:** when the dialog is gone (no `☐` prompt header, v1's marker), it's active again. This covers Esc, which fires no hook.
- **A Cursor track that's active,** in kinds that run without `--force` (Ask, Plan, Review, Doc): Cursor's approval and question dialogs mean action required, and their absence means active. Their markers are taken from a real Cursor session before this is built.

## Pull requests

- **Stored** in a `track_prs` table: track, URL, repo, number, state (`open`, `draft`, `merged`, `closed`), checked at. A Review track's reviewed PR is not one of its PRs.
- **The poll** runs in the daemon every 2 minutes, one pass for all tracks:
  - every open or draft PR, with `gh pr view <url> --json state,isDraft,number`, for tracks in any status, until it's merged or closed;
  - for each Work track that isn't closed, `gh pr list --head <branch> --state all` in each repo, which finds PRs the hooks didn't report.
- **Without `gh`,** or logged out, the poll skips its pass and logs it once. PRs from hooks still show as open.
- A merged PR changes nothing but the PR status: the track isn't ended for it.

## Storage

Migration `0004_status.sql`:
- `tracks.waiting`: the agent's dialog state, `0` or `1`, with `waiting_at`.
- `track_prs`, as above, keyed by track and URL.

## Packages

- **New: `hooks`:** the per-engine event mapping, the settings file and plugin it installs, and `tracks hook`'s payload reading. No UI imports.
- `track`: `status.go`, the two groups, `Event` and `Apply`.
- `store`: the migration, the waiting state and the PRs.
- `tracks`: `Report`, the pane check and the PR poll; Create and Resume install the hooks.
- `agents/claude`, `agents/cursor`: the `--settings` and `--plugin-dir` arguments.
- `rpc`, `daemon`: the `report` method; the poll's timer.
- `cli`: the hidden `hook` command.
- `ui/source`, `ui/tracksview`: both statuses in Station and the details; `footer`: the badge from a `@tracks_attention` window option.
- `github` (v1's, reused unchanged if it fits): `gh pr view` and `gh pr list`.

Sync before building: the new `hooks` package.

## Not in this plan

- Notifications (macOS) on action required.
- A "your turn" status for a finished turn.
- The running tool and the agent's last message as a snippet; the event timeline (chunk 4).
- Hooks talking back to the agent (review gates).
- CI checks and review state on PRs.
- A fallback for agent CLIs that don't run the hooks (v1-style screen polling).

## Tests

- **`track`:** `Apply` as a table: every event from every status, including agent events on done and closed tracks being dropped; the PR status for none, open, draft, mixed open and merged, all merged, all closed; labels for one and several PRs; every value has a label and token.
- **`hooks`:** each engine's documented payloads map to the right event, and unknown ones to none; the PR URL from `gh pr create` output and from `TRACKS_PR_URL=` lines, and not from other tool output; golden files for the Claude settings and the Cursor plugin; malformed and oversized input still exits 0; with the daemon down it returns within about a second.
- **`store`:** the migration; waiting set and cleared; PRs added once and updated.
- **`tracks`:** `Report` through a fake store; the poll with a fake `gh`: known PRs updated, a branch's new PR found, settled PRs not asked again, a missing `gh` skipped; the pane check with fixed screens.
- **`ui/tracksview` and `footer`:** Station and the details show both statuses; the badge format.
- **By hand:** real Claude and Cursor tracks: a permission prompt, a question, Esc on a dialog, a finished turn staying active, a PR opened by the agent and one by hand, a merge found by the poll.

## Delivery

Three branches, each its own PR, rebased onto the last:
1. **`feat/v2-status`:** this plan; `track` statuses and `Apply`; the migration; `Report` for created, resumed, ended and cleaned; Station and the details. Playable: statuses show for real tracks.
2. **`feat/v2-status-hooks`:** `hooks`, `tracks hook`, installing them; action required and back; the pane check; the footer badge.
3. **`feat/v2-status-prs`:** `track_prs`, PRs from hooks, the poll; docs and the masterplan.
