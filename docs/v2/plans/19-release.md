# Plan: the v2.0.0 release

**Status: planned.** Part of the [v2 masterplan](../masterplan.md), chunk 8. v2 replaces v1 in this repo: v1's code goes, `internal/v2` moves up, and `tracks` starts v2 without a flag. The [decisions](../masterplan.md#decisions) settle what's around it: no import of v1's tracks, the plain paths, a local install, Homebrew later.

## What users get

- **`tracks` is v2.** No `--new-app`, no `make dev`: the release binaries, `make build` and `scripts/install.sh` contain v2. `tracks --version` says 2.0.0.
- **The plain paths:** `~/.local/state/tracks`, `~/.config/tracks`, and the `tracks` tmux socket. v1's files there (`state.json`, `locks`, `logs`, `sentinels`, `config.yaml`) stay unused; v2's worktrees sit next to v1's in `worktrees/` and ignore them.
- **The helpers lose `-v2`:** the `tracks-reviewer` and `tracks-docs-reviewer` agents, the `tracks-add-repo` skill and `~/.cursor/rules/tracks.mdc` replace v1's files of those names. The Cursor rule applies when `TRACKS_ID` is set; `TRACKS_NEW_APP` is gone.
- **`tracks update`** replaces the binary with the latest release, as v1's does, checked against `SHA256SUMS`.
- **The README describes v2,** with a short glossary of the words the app uses (track, Station, engine, Derail, Archive and the like).

Not carried over: v1's tracks, its dashboard and its scripting commands, which the masterplan's scope already drops.

## How it works

The work runs in two PRs, so `main` keeps a working v1 release until the switch.

### 1. v2 stands alone

v2 still leans on v1 in a few places; each goes first, while v1 is there to compare with.

- **Shared packages stay:** `git`, `notify`, `shellx`, `usage` and `update` aren't v1's; they keep their place under `internal/`. `usage` stops importing v1's `state` for `state.Usage` and gets a type of its own.
- **Golden files replace the comparisons with v1:** the prompt parity test (`agents/parity_test.go`), the form's texts (`addtrack/parity_test.go`), and the window label and candor levels (`track/track_test.go`) compare with files in `testdata` written from v1 now. The `-v2` names v1's comparison swaps become the plain ones in the switch.
- **`tracks update`** in v2's CLI, a port of `cmd/update.go` onto `internal/update`. A daemon of the old version restarts on the next `tracks`, as it does after any rebuild.

### 2. The switch

- **Delete v1:** `cmd/`, `newapp.go`, `v2_on.go`, `v2_off.go`, and the v1 packages: `agent`, `claude`, `cursor`, `config`, `daemon`, `dlog`, `github`, `ports`, `provision`, `proxy`, `services`, `state`, `tmux`, `tui`. `main.go` calls v2's CLI with the version.
- **Move `internal/v2/*` to `internal/`** with `git mv`, and rewrite the imports. `internal/v2/tmux` takes the place of v1's `tmux`.
- **Names and paths:** `platform` drops `-v2` from the folders and the socket; the helpers and every prompt that names them drop it too, which brings those prompts back to v1's text word for word. The tmux config stops setting `TRACKS_NEW_APP`, and the Cursor rule stops asking for it.
- **Build:** the Makefile loses `dev` and the tag; CI loses the `-tags v2` vet. `go test ./...` covers everything again.
- **Scripts:** `install.sh` names Claude Code or Cursor as the agents it needs; `uninstall.sh` keeps offering to delete the state folder, now v2's too.
- **Docs:**
  - the README is rewritten for v2, with the glossary;
  - `AGENTS.md` describes v2's packages and checks;
  - `docs/ROADMAP.md` takes the masterplan's follow-ups and future features;
  - `docs/v2/` and v1's `docs/design/` are deleted, git history keeps them; what stays useful about a package moves into its `doc.go`.
- **The version:** the last commit is `feat!: Tracks v2 replaces v1` with a `Release-As: 2.0.0` footer. Release Please then opens the 2.0.0 release PR; merging it tags the release and builds the binaries.

## Before installing it

- **Finish v1's tracks first.** Their worktrees stay in `~/.local/state/tracks/worktrees`, but no binary can resume them after the switch. Stop v1 before installing: its daemon and tmux session keep running on the old binary until they exit.
- **The `-v2` helpers from development** (`~/.cursor/rules/tracks-v2.mdc`, `~/.claude/agents/tracks-v2-*.md`, `~/.claude/skills/tracks-v2-add-repo`) and the `tracks-v2` folders are deleted by hand. The rule never applies again without `TRACKS_NEW_APP`.

## Not in this plan

- Homebrew, and `tracks update` deferring to it: after v2.0.0.
- Removing v1's leftover files automatically: v1 was never published.
- Everything in the masterplan's "After v2.0.0" list and its follow-ups.

## Tests

- The golden files fail on any change to a prompt, a template text, the window label or the candor levels, as the comparisons with v1 did.
- `update`: v2's command finds a newer release, refuses a bad checksum and replaces the binary, against a fake release server as v1's tests do.
- `platform`: the folders and socket are the plain names, under `XDG_STATE_HOME` and `XDG_CONFIG_HOME` when set.
- The helpers install under the plain names, and the Cursor rule's condition names `TRACKS_ID` only.
- An isolated run of the release build: start, create a track on each engine, close, reopen.

## Delivery

- **PR 1, `chore/v2-stand-alone`:** this plan; `usage`'s own type; the golden files; `tracks update`.
- **PR 2, `feat/v2-release`:** delete v1; move up; names and paths; build and CI; scripts; docs; the release commit. After it merges, the Release Please PR for 2.0.0.
