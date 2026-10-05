# AGENTS.md

Tracks runs coding agents side by side, each in a track of its own: a git worktree and a tmux
window, managed from one Tracks session. The code is in `internal/`; `main.go` hands the
command line to `internal/cli`. Open work is in [docs/ROADMAP.md](docs/ROADMAP.md).

## Rules
- The installed tracks may be running: never run `make install` or a bare `./tracks` while working on Tracks. Build to a temp path, and run it with its own `HOME` and `XDG_*`.
- Tests never touch a real tmux server: use `tmuxtest.Socket(t)`. The tmux client panics in tests on any other socket.
- The database is `~/.local/state/tracks/tracks.db`. Tests use a temp file; end-to-end runs set `XDG_STATE_HOME`, and `HOME` too, since the daemon writes the reviewer subagents to `~/.claude/agents`.
- A schema change is a new file in `store/migrations/`; applied migrations are never edited.
- Colours come only from theme tokens (`theme.Token`), never colour values in code (tests may use them as data); a test enforces it. A new token goes into `theme/tokens.go` and gets a value in both built-in themes (`theme/themes/*.yaml`).
- UI uses Charm v2 (`charm.land/...`), never the v1 modules (`github.com/charmbracelet/bubbletea`, ...).
- Prompts and templates are checked against golden files in `testdata`. Changing one is a decision of its own, agreed first, never a side effect of other work.

## Map
- `cli/` commands and start-up · `platform/` paths
- `tmux/` tmux on Tracks' own socket, generated config, the prefix keys (`keys.go`) · `tmux/tmuxtest/` throwaway servers for tests
- `trackwin/` a track's window and its panes
- `theme/` design tokens, built-in themes (`themes/*.yaml`) and user theme files · `settings/` `settings.yaml` (the chosen theme, the engines) · `ui/style/` tokens to Lip Gloss colours
- `footer/` the footer's tmux status rows · `sysinfo/` LAN, WAN, CPU and memory for the footer
- `ui/tracksview/` the Tracks window (window 0) · `ui/themecreator/` theme editor, a pane of the Settings tab
- `ui/quickaccess/` the Quick Access popup (`Ctrl+b q`) · `ui/addtrack/` the New track form popup · `ui/tracksfilter/` the Tracks filter popup · `ui/confirm/` the confirm popup
- `ui/source/` the data screens read (tracks, repos, themes, engines); tracks come from the daemon
- `store/` SQLite: schema, migrations (`migrations/*.sql`, one per change), queries · `repos/` rules for adding, changing and removing repos
- `agents/` the agent CLIs (engines): finding one, its version, its models, its MCP servers, and the pane command a track runs · `agents/claude/`, `agents/cursor/` each engine's command line, prompts and session
- `hooks/` the engines' hook events and what they mean for a track's status · `notifier/` the user's notification channels
- `track/` the track domain: kinds, repos, window names · `tracks/` creating, listing, ending, resuming and cleaning tracks · `workspace/` a track's git worktrees
- `daemon/` the daemon: lock, socket, helpers, keeping the tracks in step with their windows · `rpc/` the daemon's protocol and client · `proxy/` the output ports forwarding to dev servers
- `ui/widget/` UI pieces more than one screen uses · `golden/` golden-file comparisons for tests
- Shared leaf packages: `git`, `notify`, `procs` (processes and the ports they listen on), `shellx`, `update` (`tracks update`), `usage` (token use and cost)
- Imports point downwards. Add packages here when they land.

## Build and test
- `make build` builds ./tracks · `go test ./...` · `go vet ./...`

## Conventions
- One package, one purpose. Files around 400 lines at most.
- Decisions are pure functions over plain data; tmux, git and processes sit at the edges.
- Test decisions and edges against real tools; skip glue. Each package has a doc comment.
