# Tracks roadmap

What's open after v2.0.0. Add freely; delete an item once it ships. v1's
design notes (`docs/design/`) and the v2 plans (`docs/v2/`) were deleted with
the v2.0.0 release; git history keeps them.

## Next

- **Worktree setup:** submodules.
- **Homebrew:** a formula built from the release binaries and checked against
  `SHA256SUMS`, updated by each release, depending on tmux. A brew-installed
  Tracks leaves updates to `brew upgrade`: `tracks update` and the update check
  say so instead of replacing the binary. Open: an own tap or homebrew-core.
- **Confirm Cursor's markers on a real screen:** the plan-approval and
  mode-switch markers Tracks reads from the Cursor agent CLI.

## Follow-ups

- Renaming a repo that tracks use.
- The model a track actually ran, including what Cursor's "auto" picked.
- The folder trust prompt for Ask and Plan tracks without repos.
- Cursor `create-chat` hangs with an unused `XDG_CONFIG_HOME`.
- Agents asking questions in plain text instead of their question tool.
- `NOCASE` folds only A–Z.
- Proxy and dev servers ([docs/design/proxy.md](design/proxy.md)):
  - A track window closed outside Tracks leaves its dev servers only tmux's
    SIGHUP; nothing cleans up servers orphaned by a crash.
  - Adding a repo with a setup or dev servers to a running track doesn't
    tell its agent, whose prompt has no Setup or Dev servers paragraph.
  - A server whose setup failed shows as crashed in `tracks servers` and the
    Proxy tab.
  - The Proxy tab doesn't dim a guessed server type yet.
  - `ui/tracksview/model.go` and `details.go` are over 400 lines.

## Features

- **Fast Tracks** (the Settings section is a placeholder): templates that preset
  repos, kind, engine and setup commands, stored in the database. How to start
  one is open.
- **Quick switcher** popup (`Ctrl+b s`), and more Quick Access entries.
- **Each repo's own MCP servers** (`.mcp.json`, `.cursor/mcp.json`) on the
  Engines tab.
- **Prompts:** viewing and editing the prompts each engine gets, with v1's text
  as the default.
- **Notifications:** clicking one switches to its track; platforms other than
  macOS.
- **Status:** a "your turn" status; the running tool and the last message as a
  snippet; CI checks and review state on PRs; hooks talking back (review gates);
  a screen-polling fallback for CLIs without hooks.
- **Supervision:** restarting a failing agent automatically, with a limit;
  telling a crash from a failed start.
- **Resuming an archived track,** behind a setting.
- **An event timeline** per track.
- **History:** search, filters and paging.
- **PR actions** in the track details.
