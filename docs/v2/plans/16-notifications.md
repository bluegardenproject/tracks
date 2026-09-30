# Plan: notifications

**Status: planned.** Part of the [v2 masterplan](../masterplan.md), the [v2.0.0 scope](../masterplan.md#v200-scope). A track that needs you shows it in Station and the footer, which you only see while you look at Tracks. This plan adds macOS notifications and the terminal bell, as v1 has them, for the events that matter in v2.

## What users get

- **Four events notify**, each on by default:

| Event | When | Title | Body |
|---|---|---|---|
| action required | a track's status becomes action required | Tracks: rate-bug needs you | Its agent is waiting for an answer. |
| error | a track's status becomes error | Tracks: rate-bug failed | Its agent exited with an error. |
| PR opened | a track gets a new open or draft PR | Tracks: rate-bug opened a PR | acme/api#12 |
| PR merged or closed | a PR Tracks knew as open is merged or closed | Tracks: rate-bug's PR was merged (or closed) | acme/api#12 |

- **Two channels,** each on by default: a macOS notification, and the terminal bell. The bell rings in the track's window, so tmux marks the window in the footer and passes the bell on to your terminal, which may bounce its Dock icon or flash its tab.
- **Nothing fires for the track on screen:** when a Tracks client shows the track's window, you're already looking at it.
- **Action required notifies at most once every 2 minutes per track,** as v1: an agent that asks several questions in a row doesn't flood you.
- **Settings → General → Notifications** has a toggle for each channel and each event, saved at once.

## Settings

A `notifications` section in `settings.yaml`. A missing key means on:

```yaml
notifications:
  macos: true
  bell: true
  action_required: true
  error: true
  pr_opened: true
  pr_settled: true
```

## How it works

- **The events come from two places.** `Report`, the one way a track's status changes, compares the status before and after: a change into action required or error is a notice. The PR changes come from the store: `AddPR` (the hooks) reports a new PR, and `SavePR` (the poll) now returns the state the PR had before, so a new open PR is "opened" and a change from open or draft to merged or closed is "merged or closed". A PR the poll finds already merged, such as on an old branch, doesn't notify.
- **`tracks.Service.Notify`** receives each notice: the track's ID and name, the event, a title and a body. The daemon sets it to a notifier; without one nothing is sent, which is what the tests of everything else see.
- **The notifier** (new package `notifier`) reads the settings for each notice, drops it when its event is off, when a client of the Tracks session shows the track's window (`list-clients`), or within 2 minutes of the track's last action required. Then it sends on each channel that's on:
  - **macOS:** v1's `notify` package, unchanged: `osascript`, best effort.
  - **Bell:** a BEL written to the tty of the track's agent pane (`#{pane_tty}`). v1 wrote it to the daemon's `/dev/tty`; v2's daemon has none.
- **Delivery never blocks the daemon:** each notice is sent in its own goroutine, and failures are logged once, not per notice.

## Packages

- `settings`: `Notifications`, with `On(event)` and the channel switches.
- `store`: `SavePR` returns the previous state.
- `tracks`: `Notify`, the notices from `Report`, `SeePRs` and the poll.
- **New: `notifier`:** the events, the filters, the channels.
- `tmux`: the windows shown by clients; the pane tty.
- `daemon`: builds the notifier.
- `ui/tracksview`, `cli`: the Notifications part of Settings → General.

## Not in this plan

- Clicking a notification to switch to the track: `osascript` notifications can't run anything. A helper app, or `terminal-notifier`, could later.
- Notifications on other platforms: the bell works everywhere; the rest is macOS only, as v1.
- Knowing whether the terminal itself is in front: a track on screen in a terminal behind other apps is treated as seen.

## Tests

- `settings`: defaults when the section is missing; a switched-off event and channel; saving keeps the other keys.
- `store`: `SavePR` returns "" for a new PR and the old state for a known one.
- `tracks`: notices from `Report` for a change into action required and error only; from `SeePRs` for a new PR only; from the poll for a new open PR and for open to merged, not for a PR found merged; none for an unchanged state.
- `notifier`: an event that's off, a track on screen and the 2-minute limit drop a notice; each channel is used only when on (fake senders); the bell goes to the agent pane's tty (a real tmux server).
- `ui/tracksview`: the toggles show the settings, and a toggle saves.
- **By hand:** each event with Tracks in the background, with the track on screen, and with each channel off.
