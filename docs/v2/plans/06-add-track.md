# Plan: Quick Access and the New track form

**Status: built.** Part of the [v2 masterplan](../masterplan.md): the popups of chunk 2 ([02-app-layout.md](02-app-layout.md), slice 2d) and the creation form of chunk 5. This plan builds the layout only. The form doesn't create a track yet; creating one is chunk 5.

## What users get

### Quick Access (`Ctrl+b q`)

- **Opens from anywhere:** `Ctrl+b q` in any window of a Tracks session opens a small popup centred over that window. It takes over tmux's own `q` (numbering the panes).
- **A framed list** titled Quick Access. Each entry has a key. For now there's one: **New track** (`n`).
- **Keys:** Up and down (or `j`/`k`) select, Enter or an entry's key opens it, Esc closes. Entries highlight under the mouse and open on a click, in the `listItem` colours.
- **It replaces the full menu** planned for `Ctrl+b m`: entries are added here over time, such as switching or ending a track.

### New track form

- **A larger popup**, about 80% of the window, framed and titled New track in the theme's colours. tmux 3.3 and newer draw it without their own border, on the theme's overlay background; tmux 3.2 frames it once more and keeps the terminal's background.
- **One screen.** At the top, a **Type** row: Work, Ask, Plan, Review, Doc review, one of them selected. Left and right change it while the row has focus, or a click. Under the row, v1's one-line description of the selected type.
- **The fields below change with the type,** as in v1:

  | Type | Fields |
  |---|---|
  | Work | Repos, Name, Terminal, Prompt |
  | Ask | Repos (optional), Name, Question |
  | Plan | Repos (optional), Name, Prompt |
  | Review | Repo, PR or branch, Name, Candor, Prompt |
  | Doc review | Document, Repos for grounding (optional), Name, Sections, Candor, Prompt |

- **The fields:**
  - **Repos:** a select list of the repos on the Repositories tab ([07](07-create-track.md) replaced the checkbox list). Review picks exactly one. With no repos yet: "Add a repo on the Repositories tab first."
  - **Name:** optional; it names the track's window. (v1 calls it the slug.)
  - **Terminal:** a checkbox, "Open a shell in the worktree, beside the agent."
  - **PR or branch** and **Document:** text inputs with v1's placeholders.
  - **Sections:** checkboxes for Opinion and Claim check, both on, with v1's labels.
  - **Candor:** a field that opens the picker with v1's ten levels and labels; 3 is the default. Left and right step through the levels too.
  - **Prompt** (Question for Ask): a text area of several lines, prefilled with v1's template text for the type, word for word. Work and Ask start empty. Switching the type swaps the prefill only while the prompt is untouched.
- **Values carry over** when the type changes: the repos, name and prompt a user typed stay.
- **Create and Cancel** at the bottom.
  - Create first checks what v1 requires: at least one repo for Work and Review, a PR or branch for Review, a document for Doc review, and a prompt. Problems show under their fields.
  - Then it says "Creating tracks isn't built yet." The form stays open.
  - Cancel or Esc closes; with anything typed, it asks "Discard this track?" first.
- **Keys:** Tab and Shift+Tab move between fields, Space toggles a checkbox, Enter in the prompt adds a line. The hint row lists the keys for the field with focus. Clicks focus fields, and controls highlight under the mouse. The form scrolls with the focus, or the wheel, when it's taller than the popup.
- **Runs on:** the last field, the agent and model side by side (`[ Claude Code ▾ ]  [ opus ▾ ]`). They start on the type's, from Settings → Tracks, and follow the type until the user picks others. Clicks, or Enter on the focused one, open a picker; ←/→ go between them, ↑/↓ leave the row.
  - **The agent picker** lists the agents added on the Engines tab. Picking another starts it on **Default**; picking the type's agent again brings back the type's model.
  - **The model picker** is Settings → Tracks's: **Default** ("Claude Code's default: opus"), then Claude Code's aliases and the models added on the Engines tab, or Cursor's `agent --list-models`. Cursor's list is asked for in the background when the form opens; until it arrives the picker says "Loading models…", and when it fails it says why and Enter asks again.
  - A type set to an agent that isn't added shows it with "Cursor isn't added on the Engines tab. Add it there, or pick another agent." below the fields, and Create moves the focus there instead of creating. With no agent added, the field says **None added**.

## How the popups open

- `Ctrl+b q` runs a hidden command in the background. It opens Quick Access with `display-popup -E` on the client that pressed the key and waits for it to close. When New track was chosen, it then opens the form in a second popup. tmux shows one popup per client at a time and can't resize one, so the menu and the form are two popups.
- Both popups draw with the theme in use, like the Tracks window.
- **Sizes:** Quick Access fits its entries, 40 columns wide; the form takes 80% of the window, at least 72 by 30 cells, and all of a smaller one.
- **Keys list:** `Ctrl+b q` joins `tmux.Bindings`, so the generated config and the Keys section in Settings both show it. The Keys section also lists the keys of both popups. `02-app-layout.md` drops `Ctrl+b m`.

## Texts kept from v1

The type descriptions, the template prompts, the candor levels and labels, and the section labels come from v1 (`internal/tui/newtrack`, `internal/state`); the Type row uses short names. The masterplan's decision to keep v1's prompts covers the templates, so a test reads v1's texts from its source and compares them with v2's.

## Later, not in this plan

- Creating the track: worktrees, the agent and its prompt (chunk 5).
- Resume, and starting from a Fast Track.
- More Quick Access entries.

## Packages

- **New: `ui/quickaccess`:** the menu and its entries.
- **New: `ui/addtrack`:** the form, the fields per type, and v1's texts.
- `ui/widget`: a titled frame and the hint row, which both popups draw. The checkboxes and the text area stay in `addtrack`.
- `tmux`: the `q` binding, and opening a popup on a client and waiting for it.
- `cli`: the hidden launcher and the two popup commands.

## Tests

- `quickaccess`: keys, click, what the choice returns.
- `addtrack`: switching types changes the fields and keeps values; the prefill follows the type only while untouched; required fields per type; Esc with edits asks first; Create says it isn't built yet.
- v1 parity: the template prompts, type descriptions, candor labels and section labels match v1's.
- `tmux`: the config's new binding, and the popup's flags per tmux version.
- An isolated run of both popups end to end.

## Delivery

One branch, `feat/v2-add-track`: this plan; the popup plumbing and key; Quick Access; the form; docs.
