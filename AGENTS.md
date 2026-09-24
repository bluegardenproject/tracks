# AGENTS.md

This repo holds two apps. **v1** is everything outside `internal/v2/` and ships in releases.
**v2** lives in `internal/v2/`, is built only with `make dev` (build tag `v2`) and runs with
`./tracks --new-app`. Direction and decisions: [docs/v2/masterplan.md](docs/v2/masterplan.md).

## Rules
- v2 work stays in `internal/v2/` and `docs/v2/`. v1 fixes don't touch them.
- v2 may import v1 leaf packages unchanged (`git`, `shellx`, ...). If it needs a change, copy the package into `internal/v2/`.
- The installed tracks may be running: never run `make install` or a bare `./tracks` during v2 work.
- v2 tests never touch a real tmux server; they start throwaway servers on their own sockets.
- v2 colours come only from theme tokens, never colour values in code.

## v2 map
- `cli/` commands · `platform/` paths and profiles (default, demo)
- Imports point downwards. The planned layout is in the masterplan; add packages here when they land.

## Build and test
- `make dev` builds ./tracks with v2 · `go test ./internal/v2/...` · `go vet -tags v2 ./...`
- v1's daemon tests can open windows in a live Tracks session (ROADMAP Bug 7). Don't run `go test ./...` inside one; CI runs it.

## Conventions
- One package, one purpose. Files around 400 lines at most.
- Decisions are pure functions over plain data; tmux, git and processes sit at the edges.
- Test decisions and edges against real tools; skip glue. Each package has a doc comment.
