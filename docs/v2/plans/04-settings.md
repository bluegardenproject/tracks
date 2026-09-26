# Plan: Settings tab and theme files

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 7 (Tracks window content). The Settings tab gets real content, and the theme creator moves into it. Themes become files, one per theme, and light mode goes: a light look is simply another theme.

## What users get

- **Two frames:** on the left quarter, "Settings" with one stacked button per section (like the Repositories tab's New button); on the other three quarters, the selected section under its name.
  - Up and down (or `j`/`k`) and clicks choose a section. Enter or a click inside moves focus into it; Tab and Shift+Tab move within; Esc goes back. Both frames use `border.default`.
  - Narrow windows show whichever frame has focus.
- **General:** preferences. For now one, **Theme**: a field showing the file in use (`[ default.yaml ▾ ]`) and its display name. Enter or a click opens the **theme picker**.
- **Theme picker:** an overlay centred on the window, listing every theme file by file name with its display name beside: the built-ins first, then the themes folder's files by name. The one in use is marked. Choosing one applies it everywhere at once and remembers it; Esc or a click outside closes it. Files that aren't valid themes are listed greyed out with the reason, and can't be chosen.
- **Fast Tracks:** second in the list. Until Fast Tracks are built it shows the empty state: a " + New " button, a line on what Fast Tracks are, and a centred, framed "Create your first Fast Track now" button. Both buttons are placeholders that say Fast Tracks aren't built yet; the section doesn't take focus.
- **Theme Creator:** edits a theme.
  - It opens on the current theme; **Load** picks another in the same picker, marking the one being edited.
  - One row per token: name, preview, value. The preview shows the draft; the creator's own colours stay on the applied theme, so a bad edit can't make it unreadable.
  - Buttons: **Save** (user themes only; re-applies it if it's the current one), **Save as new** (asks for a display name, writes a new file), **Copy all** (the file's text to the clipboard).
  - Built-ins are read-only: saving one means Save as new.
  - Unsaved edits ask Save, Discard or Cancel when leaving, as on the Repositories tab.
- **Keys:** every shortcut, read-only and scrollable, grouped: Tracks window, Station, Repositories, Settings, Theme picker, Theme Creator, and the prefix keys every window has (`Ctrl+b …`).
- **About:** version, profile, and where things are: config folder, themes folder, settings file, data folder, database, tmux socket.
- **Gone:** the theme creator overlay and its `t` key; the terminal background detection (`footer system --light`); the swatch grid.

## Theme files

- **Format:** `display_name`, what Tracks shows for the theme (the id when it's missing), and one value per token.

  ```yaml
  display_name: Default
  tokens:
    text.default: "#e4e4e7"
    ...
  ```

- **Built-ins,** embedded and read-only, listed as if they were files: **Default** (`default.yaml`, the dark values) and **Default Light** (`default_light.yaml`). The themes folder holds only user files.
- **User themes:** `~/.config/tracks-v2/themes/<id>.yaml`, the id being the file name. Save as new turns the display name into the id (`My Theme` → `my_theme.yaml`, like the built-ins) and refuses one that exists.
- **Valid:** YAML with known tokens and `#rrggbb` values. Tokens a file lacks, such as ones added later, come from Default. An unknown token, a bad value or an id a built-in has makes the file invalid.
- **The choice** is `theme: <id>` in `~/.config/tracks-v2/settings.yaml`, shared by `--new-app` and `--demo`. A missing or invalid choice falls back to Default.
- Every process that draws (Tracks window, footer, track windows) reads the choice and its theme. `<data>/theme.yaml` is no longer read; nothing was released, so there's no migration.

## Keys, defined once

The prefix bindings are a list in `tmux` (`tmux.Bindings`: keys, label, help, command). The generated tmux config and the Keys section both read it, so they can't disagree. The Tracks window's own keys are lists in `ui/tracksview/keys.go`, read by both the hint row and the Keys section.

## Packages

- `theme`: the single-value format; built-ins; `List(dir)` returns every theme with id, display name, built-in or path, and why it's invalid; `Load(dir, id)`; `Save`.
- **New: `settings`**: reads and writes `settings.yaml` (for now just `theme`). Unknown keys are kept on save, so older and newer builds can share the file.
- `ui/themecreator`: a pane the Settings tab embeds at the size it's given, instead of a full-screen overlay; loads and saves through the host.
- `ui/widget`: `Picker`, the framed list the theme picker draws; it knows nothing of themes.
- `ui/tracksview`: `settings.go` (state, keys, the creator's requests), `settingsview.go` (frames, sections, clicks) and `keys.go` (the key lists).
- `cli`: resolves the theme from settings at start, and implements `source.Themes` for the window: list, choose (saves the setting, rewrites `theme.conf`, reloads tmux), save and create.

## Tests

- `theme`: parsing, filling missing tokens, invalid files with their reason, listing built-ins before files, a file that takes a built-in's id.
- `settings`: round trip, missing file, unknown keys kept.
- `tracksview`: choosing a theme in the picker applies and saves it; an invalid theme can't be chosen, and a click outside closes the picker; the creator's Load goes through the picker; saving through the host; unsaved edits ask before leaving.
- `ui/themecreator`: saving, Save as new on built-ins, leaving with unsaved edits, loading, the mouse.
- `tmux`: the config's bindings come from the list (golden files).

## Delivery

One branch, `feat/v2-settings`: theme files and built-ins; `settings`; keys list; creator as a pane; the Settings tab; docs.
