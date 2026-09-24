# Chunk 2: global app layout

Part of the [v2 masterplan](../masterplan.md). Builds on [chunk 1](01-technical-groundwork.md). This file is deleted in the PR that completes the chunk.

## Outcome

The demo session feels like the real app, all on fake data:
- a navigation footer on every window
- track windows with agent, terminal and dev-server panes that can be added and closed
- a simple Tracks window
- the full menu popup
- the quick switcher
- a "New track" form

The goal is to judge the layout by using it, before any real logic exists.

Out of scope: the Tracks window's header and tabs (chunk 3), real tracks (chunk 5), storage and hooks.

**Prerequisite:** the [track status model](../masterplan.md#track-status-to-be-designed-before-chunk-2). The status names and colours below are placeholders until it's designed.

## Data: the `Source` interface (`internal/v2/ui/source`)

Every screen and the footer read through one small interface:

```go
type Source interface {
	Tracks() []TrackView                  // creation order
	Watch(ctx context.Context) <-chan Change
}
```

- `demo.Source` is the implementation now: fake tracks from `internal/v2/demo`, and a timer that changes statuses every 20 to 30 seconds, so attention highlighting can be watched. A daemon-backed source replaces it in chunk 5; the UI doesn't change.
- `TrackView` is plain display data: number, name, kind, status (as defined by the status model), repo names and window ID.

## Styles (`internal/v2/ui/style`)

- Builds on the design tokens from chunk 1 (`internal/v2/theme`). New tokens are added there as screens need them.
- Adds the Huh v2 form theme, built from tokens.
- Status colours come from the status model, which names a token for each status. The footer turns token values into tmux colours.
- Library-neutral helpers (control-character stripping) sit in a package without Charm imports, so v1 can share them later if wanted.

## Track windows

Built in chunk 1 (`internal/v2/trackwin`, see [chunk 1](01-technical-groundwork.md#1d-demo-session)): the agent pane on the left, the right column at 30% with terminal and dev-server panes, titles on the top border, and adding a terminal (`Ctrl+b t` for now).

- **Switching to a track focuses its agent pane.**
- **Closing panes:** whether `exit` in the last terminal is enough, or a key is needed, is decided in the playground.

## Footer (`internal/v2/footer`)

One fixed row, the tmux status line, on every window, the Tracks window included.

```text
 ⌂ Tracks │ «  ‹ ●1 │ 3 api-auth │ 4 ui-login │ 5 docs-fix │ 6 billing │ 7 infra │ › » │ ▶ 2 dev  ⇄ proxy │ 192.168.1.20 · 85.14.3.9
```

- **Tracks** (window 0) is pinned on the left.
- **Track slots:** as many as fit the width, each with its number, status colour and name (truncated with `…`, at most 16 characters). They follow the active track like a list cursor: it's always visible, and the row shifts only when you move past the edge.
- **Buttons:** `«` first, `‹` previous, `›` next, `»` last.
- **Off-screen attention:** tracks that need attention but are scrolled out of view show a badge on that side, for example `‹ ●1`.
- **Stable order:** creation order, never reordered on status changes.
- **Colours** (placeholders until the status model is designed):
  - active: accent background
  - your turn: highlight
  - needs approval: fail colour, bold
  - running: normal
  - finished: dim
- **Info on the right:** running dev servers, proxy state, LAN IP, WAN IP. On narrow terminals these drop in the order WAN, LAN, proxy, dev, before track slots shrink below 3. In the demo, the dev-server and proxy info is fake, while LAN is read from the network interfaces.
- **WAN IP** is fetched once at start-up (HTTPS request to a configurable URL, default `https://api.ipify.org`, 3s timeout). It's omitted on failure, and an empty URL in the config turns it off.
- **No hover:** the tmux status line can't do it. Whether hover is worth a Bubble Tea footer pane per window is decided after trying this.

### How it works

- **Rendering:** `footer.Render(tracks, activeWindow, width, info, theme) string` is pure. It returns a tmux format string:
  - `#[range=window|@7]…#[norange]` per track slot
  - `#[range=user|first]`, `prev`, `next`, `last` for the buttons
  - it escapes tmux format characters in names (`#`, `[`)
- **Refresh:** `tracks footer refresh` (a hidden command) reads the source, renders for the narrowest client, stores the result in the session option `@tracks_footer`, and runs `refresh-client -S`. The generated config sets `status-format[0]` to `#{E:@tracks_footer}`.
- **Triggers, no polling:**
  - tmux hooks run `run-shell -b "<tracks> footer refresh"` on `session-window-changed`, `client-resized`, `window-linked` and `window-unlinked`
  - the demo source's status changes trigger it too
- **Clicks:** a `MouseDown1Status` binding checks `#{mouse_status_range}`. Window ranges select the window. User ranges run `tracks footer nav <first|prev|next|last>`, which selects the target window and its agent pane.

### Keys (no prefix, in the generated config)

**To be revised before 2b:** these collide with keys agents and shells use, see "Keys" in the masterplan's open questions.

- `Alt+1` to `Alt+9`: track by number
- `Alt+0`: the Tracks window
- `Alt+,` and `Alt+.`: previous and next track
- `Alt+<` and `Alt+>`: first and last track
- `Alt+s`: quick switcher
- `Alt+p`: full menu
- `Alt+t`: add a terminal pane
- `Alt+[` is avoided because it collides with terminal escape sequences.
- **macOS:** the terminal must send Option as Alt. The README says how.

## Tracks window, placeholder (`internal/v2/ui/tracksview`)

A simple first version, just enough to navigate. Chunk 3 designs the real header and tabs.

- The Tracks banner, then a list of the demo tracks with status.
- Enter switches to the track's window; `n` opens the New track form.
- Scrolls with keys and the mouse wheel. Mouse clicks select rows.

## Popups (`internal/v2/ui/menu`, `internal/v2/ui/switcher`)

- **Full menu (`Alt+p`):**
  - the menu rebuild from the `tracks/09ad3c-menu` branch, ported to Charm v2
  - sections, a filter, shortcuts, breadcrumbs, the track picker table and confirm dialogs
  - in the demo, actions act on fake tracks: switch, end a track, "new" opens the form
- **Quick switcher (`Alt+s`):** a filterable list of tracks with status. Enter switches, Esc closes. Built for more tracks than `Alt+1..9` covers.
- **New track form (Huh v2):** kind, repos, name, agent. In the demo it creates another fake track window, and the footer picks it up.
- **Size:** popups open with `display-popup -E` at about 80% of the window. The playground decides the final size.

## What the playground should decide

- Footer: slot width, colours, buttons, info segments, whether hover is worth a pane.
- Track window: right column width, pane titles, how adding and closing terminals feels.
- Popups: size, full menu vs quick switcher, the creation form.
- What the Tracks window needs, as input for chunk 3.

## Tests

- **Golden tests for `footer.Render`:**
  - widths from 60 to 240 columns
  - active track at the start, middle and end
  - badges on both sides
  - truncation and escaping
  - segment dropping order
- **Pure navigation logic:** first, previous, next and last with wrap-around off, and number keys past the end.
- **Throwaway tmux server:** the rendered string is accepted, a window range click selects the window, and the navigation keys select the right window.
- **Screens:**
  - a few golden snapshots (Tracks window placeholder, menu root, switcher with filter)
  - behaviour tests for the ported menu (navigation, filter, confirm, mouse click), carried over from the v1 menu tests

## PR slices

1. **2a: Huh styles, `Source` and demo source.**
2. **2b: footer renderer, refresh, hooks, clicks, navigation keys.**
3. **2c: footer info segments and attention badges.**
4. **2d: Tracks window placeholder, full menu popup, quick switcher, New track form.**

After each slice the maintainer plays with it, and we adjust before the next.
