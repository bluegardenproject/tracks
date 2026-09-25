# Chunk 3: Tracks window layout

Part of the [v2 masterplan](../masterplan.md). Builds on [chunk 2](02-app-layout.md), whose data interface, popups and New track form are finished after this chunk. This file is deleted in the PR that completes the chunk.

## Outcome

The Tracks window gets its layout: a banner, tabs and a hint row. Each tab shows a placeholder saying what it will hold; chunk 7 fills them.

```text
    __________  ___   ________ _______
   /_  __/ __ \/   | / ____/ //_/ ___/
    / / / /_/ / /| |/ /   / ,<  \__ \
   / / / _, _/ ___ / /___/ /| |___/ /                         v2 dev build
  /_/ /_/ |_/_/  |_\____/_/ |_/____/                             <version>

  ╭───────────╮ ╭────────────────╮ ╭─────────╮ ╭───────────╮ ╭────────────╮
  │  Station  │ │  Repositories  │ │  Proxy  │ │  Engines  │ │  Settings  │
  ╰───────────╯ ╰────────────────╯ ╰─────────╯ ╰───────────╯ ╰────────────╯

                            (tab placeholder)

  Tab next tab · Shift+Tab previous tab · t theme creator · Ctrl+b n next track
```

## Parts (`internal/v2/ui/tracksview`)

- **Banner:** "TRACKS" in figlet's Slant font, bold. Each line gets one colour of a gradient from `banner.top` to `banner.bottom`. The build and version sit on the right of its last two lines. Short windows drop the banner first.
- **Tabs,** in this order:
  - **Station:** active tracks, with status and actions
  - **Repositories:** the repositories tracks are created from
  - **Proxy:** dev servers and the local proxy
  - **Engines:** the agent CLIs that tracks run
  - **Settings:** Tracks settings. For now it shows the theme swatches; `t` opens the theme creator.
- **Boxed tabs:** every tab in a rounded box of its own, with no frame around the content. The active tab is filled. Tokens: `tab.text` and `tab.border`, and `tab.active.bg`, `tab.active.text` and `tab.active.border`. A folder-style variant (tabs open towards a content frame) was tried and dropped.
- **Switching:** Tab and Shift+Tab, wrapping around. Station is the start tab.
- **Hint row:** one row at the bottom with the keys.

Out of scope: tab content (chunk 7), and a header summary of track statuses (after the status model).

## Tests

- Tab and Shift+Tab move through all tabs and wrap around.
- The layout fits the window: at a few sizes and on every tab, every line is exactly the window's width and the view has exactly its height.
- The banner is dropped on short windows.
- Settings still shows every token.

## PR slices

One PR, together with chunk 2's navigation keys.
