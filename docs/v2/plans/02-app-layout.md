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

**Statuses are placeholders** in this chunk. The [track status model](../masterplan.md#track-status-to-be-designed) is designed later, before chunk 3 or after it.

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

- **Switching to a track focuses its agent pane,** with keys and footer clicks alike (`trackwin.Switch`).
- **Closing panes:** whether `exit` in the last terminal is enough, or a key is needed, is decided in the playground.

## Footer (`internal/v2/footer`)

Four tmux status rows on every window, the Tracks window included. Each row is one terminal line; tmux can't change a row's height.

```text
 Tracks                  ‹  1 api-auth   2 ui-login   3 docs-fix  ›
                                                                  (empty)
                                                                  (reserved for later use)
 Tracks <version>        LAN 192.168.1.20  ·  WAN 85.14.3.9  ·  CPU 12%  ·  MEM 9.1 / 16.0 GB
```

- **Navigation row:**
  - Tracks (window 0) is pinned on the left.
  - The tracks are centred, using tmux's own window list (`#{W:…}` with `list=`). When they don't fit, it scrolls with `‹` `›` markers and keeps the current track visible.
  - Clicking a slot selects its window.
- **Rows 2 and 3** are empty for now.
- **System row:** the Tracks version on the left. On the right: LAN, WAN, CPU and memory, from the hidden command `tracks footer system`, which tmux runs every 5 seconds (`#(…)`, asynchronous).
  - LAN is the address of the default route's interface. CPU and memory come from gopsutil, which needs no cgo.
  - WAN is fetched from `https://api.ipify.org` and cached for 10 minutes in the data directory. The last known address is used on failure.
- **No track status** in the footer for now.
- **Colours:** the footer has its own tokens (`footer.*`). Window and border backgrounds use `bg.base`.
- **Generated from the theme:** the rows and colours are written to `theme.conf` in the data directory, which the main config sources. Applying a theme rewrites and re-sources it, so the change is live.

Not built yet, from the first design: `«` `‹` `›` `»` buttons, badges for off-screen tracks that need attention, status colours, truncating long names, and dropping info on narrow terminals. If tmux's window list can't do these, the navigation row gets its own pure renderer, with window ranges for slots and user ranges for buttons. Hover isn't possible in tmux's status rows; whether it's worth a Bubble Tea footer pane per window is still open.

## Theme editor (`internal/v2/ui/themecreator`)

Opened with `t` in the Tracks window.

- Lists every token with a preview (text, box or colour) and hex fields for its dark and light values. Works with keys and the mouse.
- **Apply** saves `theme.yaml` in the data directory and applies it live, to the Tracks window and the tmux config. **Cancel** closes. **Copy all** copies the theme as YAML.
- Later: saving named theme files that users can share or pick.

### Keys (behind the prefix, in the generated config)

Most replace tmux defaults that don't fit Tracks. The keys run the hidden `trackwin nav` command; `trackwin.Destination` decides where they go.

- `Ctrl+b 0`: the Tracks window; `Ctrl+b 1` to `9`: track by number, nothing past the last track (built)
- `Ctrl+b p` and `Ctrl+b n`: previous and next track, skipping window 0 and not wrapping around. From the Tracks window, `n` goes to the first track and `p` to the last (built)
- `Ctrl+b <` and `Ctrl+b >`: first and last track, instead of tmux's window and pane menus (built)
- `Ctrl+b s`: quick switcher, instead of tmux's session tree (comes with the popups)
- `Ctrl+b m`: full menu, instead of marking a pane (comes with the popups)
- `Ctrl+b t`: add a terminal pane (built in chunk 1)

## Tracks window, placeholder (`internal/v2/ui/tracksview`)

A simple first version, just enough to navigate. Chunk 3 designs the real header and tabs.

- The Tracks banner, then a list of the demo tracks with status.
- Enter switches to the track's window; `n` opens the New track form.
- Scrolls with keys and the mouse wheel. Mouse clicks select rows.

## Popups (`internal/v2/ui/menu`, `internal/v2/ui/switcher`)

- **Full menu (`Ctrl+b m`):**
  - the menu rebuild from the `tracks/09ad3c-menu` branch, ported to Charm v2
  - sections, a filter, shortcuts, breadcrumbs, the track picker table and confirm dialogs
  - in the demo, actions act on fake tracks: switch, end a track, "new" opens the form
- **Quick switcher (`Ctrl+b s`):** a filterable list of tracks with status. Enter switches, Esc closes. Built for more tracks than `Ctrl+b 1..9` covers.
- **New track form (Huh v2):** kind, repos, name, agent. In the demo it creates another fake track window, and the footer picks it up.
- **Size:** popups open with `display-popup -E` at about 80% of the window. The playground decides the final size.

## What the playground should decide

- Footer: slot width, colours, buttons, info segments, whether hover is worth a pane.
- Track window: right column width, pane titles, how adding and closing terminals feels.
- Popups: size, full menu vs quick switcher, the creation form.
- What the Tracks window needs, as input for chunk 3.

## Tests

- **Footer:** the generated tmux config as golden files, the system row with and without data, and the WAN cache. If the navigation row gets its own renderer: golden tests over widths from 60 to 240 columns, the active track at the start, middle and end, badges, truncation and escaping.
- **Pure navigation logic:** first, previous, next and last with wrap-around off, and number keys past the end.
- **Throwaway tmux server:** the rendered string is accepted, a window range click selects the window, and the navigation keys select the right window.
- **Screens:**
  - a few golden snapshots (Tracks window placeholder, menu root, switcher with filter)
  - behaviour tests for the ported menu (navigation, filter, confirm, mouse click), carried over from the v1 menu tests

## PR slices

1. **2a: Huh styles, `Source` and demo source.**
2. **2b: footer rows, navigation list, system row, theme editor** (done), then the navigation keys (done).
3. **2c: footer buttons and attention badges,** once statuses exist.
4. **2d: Tracks window placeholder, full menu popup, quick switcher, New track form.**

After each slice the maintainer plays with it, and we adjust before the next.
