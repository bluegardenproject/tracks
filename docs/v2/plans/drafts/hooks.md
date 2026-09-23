# Draft: agent hooks instead of screen polling (chunk 6)

**Status: draft.** This is worked out in design, but not yet a feature plan. Part of the [v2 masterplan](../../masterplan.md).

Claude Code and Cursor report their own state through hooks. One direction for now: agent to Tracks. Talking back (injecting context, holding a Stop until the review gate passes) is a later step.

## What v1 polls today

v1's `watchTrackProcess` ([`internal/daemon/supervisor.go`](../../../../internal/daemon/supervisor.go)) ticks every 2 seconds per track:
- Liveness: the exit sentinel file, then the pane PID.
- `capture-pane`: Running vs Waiting. Waiting means the pane was unchanged for 6 seconds, which flickers with spinners, hence a 2-minute notification cooldown.
- Awaiting-input: the `☐ Confirm` marker.
- A pane-tail snippet for the dashboard.
- `TRACKS_PR_URL=` markers scraped from the screen.
- `git branch` and transcript usage every few ticks.

## What users get

- **Status changes the moment the agent acts:** no delay, no flicker.
- **Two waiting states:** **needs approval** (notified immediately) and **your turn**.
- **Clean snippets:** the current tool while running (for example `Bash: go test ./...`), and the agent's final message after a turn.
- **PR URLs come from the final message,** so a marker that merely appears on screen no longer counts.

## Event mapping

**Claude Code:**
- `SessionStart`: marks hooks healthy and records `transcript_path`.
- `UserPromptSubmit`, `PreToolUse`: Running. `PreToolUse` also sets the activity text.
- `PermissionRequest`, and `Notification` with `permission_prompt` or `elicitation_dialog`: needs approval, and a notification.
- `PostToolUse`, `PostToolUseFailure`, `PermissionDenied`: Running, and approval cleared.
- `Stop`: your turn. The snippet comes from `last_assistant_message`, which is also scanned for `TRACKS_PR_URL`. Branch and usage refresh here.
- `StopFailure`: your turn, with the error shown.
- `SessionEnd`: agent exited. The sentinel file stays as the crash backstop.

**Cursor:**
- `sessionStart`: hooks healthy.
- `beforeSubmitPrompt`: Running.
- `postToolUse`, `postToolUseFailure`: Running, with activity text.
- `afterAgentResponse`: sets the snippet, and it's scanned for `TRACKS_PR_URL`.
- `stop`: your turn. Its `aborted` status covers an interrupt, and `error` is shown.
- `sessionEnd`: agent exited.
- **Not subscribed:** `preToolUse` and the `before*Execution` hooks. They are permission hooks, where wrong output blocks the action, and `allow` would bypass the user's approval.

## Installation, per track, never in global files

- **Command:** `'<abs path>/tracks' hook --provider <claude|cursor> --track <id>`, with a 5-second timeout. The track ID is baked into the command, so it doesn't depend on environment inheritance.
- **Claude:** a per-track `claude-settings.json` passed with `--settings <path>`. Claude merges these hooks with the user's own.
- **Cursor:** a per-track local plugin passed with `--plugin-dir <path>`. It contains `.cursor-plugin/plugin.json` (`{"name": "tracks-hooks"}`) and `hooks/hooks.json`. This needs a Cursor CLI from the August 2026 release or later.
- The per-track hooks directory is deleted when the track is finalized. `~/.claude/settings.json` and `~/.cursor/hooks.json` are never edited.

## `tracks hook` (hidden command, `internal/v2/hooks`)

- Reads stdin (capped at 1 MiB) and keeps only the mapped fields, truncated. The daemon strips control characters.
- Sends one `hook` request to the daemon socket, with a 1-second timeout.
- It never affects the agent: it always exits 0, prints nothing for Claude and `{}` for Cursor, and logs failures to `<data>/logs/hook.log`.
- It runs synchronously, so events arrive in order. The cost is about 10 ms per tool call.

## Daemon side

- **Protocol:** a `hook` method with `{TrackID, Provider, Event, At, ToolSummary, Message, NotificationType, StopStatus, TranscriptPath}`.
- **State function:** a pure `applyHook(track, event) (track, effects)`. Effects are notify, start the PR watcher, refresh branches, refresh usage. Every accepted event is also appended to the timeline ([storage draft](storage.md)).
- **Supervision** (`internal/v2/supervise`): `hookMode` is `pending`, `hooks` or `legacy`. The 2-second tick keeps only liveness (sentinel file and `kill -0`).

## Fallbacks

- **No hook event within 20 seconds of spawn:** `legacy` mode, which is v1-style screen polling. Causes include an old CLI, `disableAllHooks`, `allowManagedHooksOnly`, or a blocked Cursor plugin. It's logged once and shown in the track details.
- **Claude interrupt (Esc):** `Stop` doesn't fire. While Running with no hook for 30 seconds, check the pane; unchanged for 6 seconds means your turn.
- **Claude approval of a long command:** between `PermissionRequest` and `PostToolUse`, a pane check notices when the approval dialog is gone.
- **Cursor approval prompts:** these only happen in kinds without `--force` (plan, ask, doc). For those, keep the idle and `☐` check at a 6-second cadence while Running.

## Tests

- **`applyHook` table tests per provider:**
  - prompt, tool, permission, tool, stop
  - interrupt, then fallback
  - `StopFailure`, and Cursor `stop` with `aborted`
  - duplicate and unknown events
  - PR marker in the final message vs inside tool output
- **Golden files** for the Claude settings file and the Cursor plugin.
- **`tracks hook`:**
  - documented payload fixtures
  - malformed and oversized input still exit 0
  - with the daemon down, it exits within about 1 second
- **Manual:**
  - real Claude and Cursor tracks: status changes, approval, interrupt, PR marker, resume
  - `disableAllHooks` falls back to `legacy`

## Open risks

- **Payload drift across CLI versions:** only named fields are read, and a missing field degrades to `legacy`.
- **Cursor `--plugin-dir` trust:** Cursor's docs don't say whether it needs a trust step in interactive mode, or whether an Enterprise local-plugins policy blocks it. This must be verified by hand.
