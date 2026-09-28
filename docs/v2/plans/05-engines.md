# Plan: Engines tab

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 7 (Tracks window content). The Engines tab sets up the agent CLIs tracks run on, Claude Code and Cursor, so that chunk 5 (real tracks) can start them. Nothing here starts an agent yet.

## What users get

- **One framed box per engine,** full width, titled with the engine's name: **Claude Code**, then **Cursor**. Both frames use `border.default`.
- **Empty state:** until an engine is added, its box holds only a centred, framed button, "Add Claude Code to Engines" (like Fast Tracks' "Create your first Fast Track now").
- **Adding checks the CLI:** the button looks for the program on the `PATH` of the Tracks tmux server, the one tracks will start in, and asks it for its version.
  - Found: the engine is added and saved.
  - Not found: the box says why ("`claude` isn't on the PATH.") with an install hint and a **Check again** button. Nothing is saved.
- **An added engine** shows, top to bottom:
  - **Status:** a badge, green **active** (`state.success.bg.accent`), or red **not found** (`state.danger.bg.accent`) once the CLI is gone, with **Check again** and the install hint; **checking** while it looks. Beside it the program's path and version.
  - **Default model:** a field like the theme field (`[ opus (latest) ▾ ]`). Enter or a click opens the model picker. The first entry is the engine's own default, which passes no `--model`; it's what an engine without a choice uses.
  - **Models (Claude Code only):** Claude has no command that lists models, so the list is the four aliases that follow each family's newest release, **Opus (latest)**, **Sonnet (latest)**, **Haiku (latest)** and **Fable (latest)**, then the user's own ids. An input and an **Add model** button add an id such as `claude-opus-5-5`; each added id has a remove button. The default model is picked from this list.
  - **Cursor's models** come from `agent --list-models`, the models the account can use. They're read when the picker opens (a network call, so it shows "Loading models…", and an error that Enter retries if it fails), and kept until the window closes.
  - **MCP servers:** a **Check MCP servers** button runs `claude mcp list` or `agent mcp list` and lists each server with its status in the CLI's words, green when connected or ready, yellow when it needs approval or a login, red when it fails. It runs outside any repo, so only servers set up for every project show, and it connects to each one, so it runs on request (with "Connecting to each server…") rather than when the tab opens. The list stays until the window closes. Cursor's box adds that servers from Cursor plugins (such as Figma) aren't listed: the Cursor CLI only shows them in `/mcp list` inside a session, and has no command that lists them.
  - **Auto mode:** on by default, as in v1. A **Disable auto mode** button (then **Enable auto mode**) with the hint: "Without auto mode the agent asks before running commands or changing files, so tracks wait for you more often and work slows down. Ask, plan and doc tracks keep their own modes."
  - **Remove:** a danger button at the bottom. It asks first ("Remove Claude Code from Engines? Its model settings are removed too.") with **Remove** and **Cancel**, like deleting a repo.
- **Checked again** every time the tab opens, so an uninstalled or moved CLI shows **not found**.
- **Keys:** Enter or a click moves focus into the tab; Tab and Shift+Tab move between the controls of both boxes; Esc goes back. The content scrolls when the boxes don't fit.
- **The model picker** is the theme picker's overlay, with one addition: typing filters the list by id and name (Cursor lists many models), so the arrow keys move the cursor. The CLI's labels are drawn without control or zero-width characters.

## What auto mode means

It maps onto what v1 does, for work and review tracks only:

| Engine | Auto mode on (default) | Off |
|---|---|---|
| Claude Code | `--permission-mode auto` | `--permission-mode default` |
| Cursor | `--force` ("Run Everything") | no `--force` |

Ask and plan tracks keep their read-only modes, and doc reviews keep v1's prompting mode, whatever this says.

## Settings

The `engines` section of `~/.config/tracks-v2/settings.yaml`. An engine is added when its key is there; Remove deletes the key.

```yaml
theme: default
engines:
  claude:
    model: opus            # empty: Claude's own default
    models: [claude-opus-5-5]
    auto: false          # only written when off
  cursor:
    model: gpt-5.3-codex   # empty: Cursor's own default
```

The program is always found on `PATH` (`claude`, `agent`); there's no path override for now.

## Later, not in this plan

- The engine new tracks use by default, and per-kind default models (work, review, ask, plan, doc): with the creation form (chunk 5).
- The global helpers v1 installs (reviewer agents, add-repo skill, Cursor rule): chunk 5.
- Each repo's own MCP servers (`.mcp.json`, `.cursor/mcp.json`), and a server's tools (`agent mcp list-tools`): with repos and tracks (chunk 5).
- A view of the prompts each engine gets, and editing them: after chunk 5. The prompts stay v1's ([Decisions](../masterplan.md#decisions)).

## Packages

- **New: `agents`:** the engines Tracks knows (name, program, how to read the version and the models), finding a CLI, `mcp list` parsing for both CLIs, and `agent --list-models` parsing, copied from v1's `internal/tui/newtrack/provider.go`. No UI imports. Chunk 5 adds spawning here, as the masterplan's draft layout already plans (`agents/`, then `claude/` and `cursor/`).
- `settings`: the `engines` section, keeping unknown keys as today.
- `ui/source`: an `Engines` interface: check, add, remove, save an engine's settings, list Cursor's models.
- `ui/widget`: `Picker` gets type-to-filter and a loading and error state.
- `ui/tracksview`: `engines.go` (state, keys, requests) and `enginesview.go` (boxes, clicks).
- `cli`: implements `source.Engines` for the window.

## Tests

- `agents`: finding a CLI and reading its version, with fake programs in a temp `PATH`; a missing one; `--list-models` parsing, including v1's cases; `mcp list` parsing on both CLIs' output.
- `settings`: the `engines` section round trip; removing an engine keeps the rest of the file.
- `ui/widget`: filtering, and the cursor staying on a visible row.
- `tracksview`: adding with the CLI found and not found; the badge turning to not found on re-check; picking a default model, including the engine's own default; adding and removing a Claude model id; the auto mode toggle; Remove asks first; checking the MCP servers, and a failed check.

## Delivery

One branch, `feat/v2-engines`: this plan; `settings` engines section; `agents`; the picker's filter; the Engines tab; docs.
