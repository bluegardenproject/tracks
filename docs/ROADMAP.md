# Tracks roadmap & backlog

Single place to collect ideas, planned work, and small tasks for the Tracks
CLI. Add freely; delete things once implemented/fixed.

**How this is organised:**
- **Big topics** — larger efforts with their own phasing (v1…vX). Each has a
  short summary here and links to a detailed design doc under `docs/design/`.
- **Small tasks** — a flat checklist of self-contained improvements/fixes.
- **Ideas / parking lot** — not yet committed to; raw thoughts.
- **Recently shipped** — move done items here briefly, then delete.

---

## Big topics

> Priority marker: ⭐ = high priority (do first / blocks other work).

### 0. ⭐ Worktree provisioning (deps + gitignored env)  *(blocks Topic 1)*
A fresh worktree isn't runnable: no `node_modules`, no gitignored env/config.
Until this exists, dev servers / builds / tests can't run in a track. Caching
is essential (huge repos). Generic `provision:` config block; Ledger specifics
in config.
**Detail:** [`design/worktree-provisioning.md`](design/worktree-provisioning.md)

- [x] **v1** — `cache_strategy`: `apfs-clone` (COW) for the non-pnpm case.
      **Shipped** (`7d91e10`): clonefile(2) COW-clone of the primary's
      `node_modules` before the deps install, with a plain-copy fallback off
      APFS/across volumes.
- [ ] **v2** — async provisioning + "provisioning→ready" status; auto-detect
      package manager.

### 1. Dev servers & autonomous testing inside a track
Run/verify branch changes without releasing them — dev servers for LLD + live
apps, and autonomous mobile testing via Argent.
**Detail:** [`design/dev-servers.md`](design/dev-servers.md) ·
[`design/argent-spike.md`](design/argent-spike.md)

- [x] **v1a** — LLD + live apps, human drives.
      **Shipped:** config, ports, supervisor, readiness, hooks, group-kill
      teardown (PRs #14–#17), then CLI control surface + tmux viewer panes +
      service_ready notification + stable-port reverse proxy (this PR).
      `tracks up/down/services/url/proxy` are all working.
- [ ] **v1b** — mobile manual (Metro + simulator boot, hand off to human).
- [ ] **v1c** — autonomous mobile smoke-test via Argent (isolation + MCP
      injection + task-suffix). *Next: run the Argent spike.*
- [ ] **v2** — full autonomous verify loop (Playwright/-MCP for LLD, Argent for
      mobile), gated into the `tracks-reviewer` pre-PR check.

### 2. Shareability (use beyond this machine / with the team)
Keep the binary generic; make config portable so teammates can adopt it.
**Detail:** [`design/dev-servers.md` §6](design/dev-servers.md)

- [ ] `${VAR}` / env expansion + relative paths in `config.yaml`.
- [ ] Checked-in template config + hook scripts as a `tracks-ledger-setup`
      starter repo (clone, set `LEDGER_ROOT`, run).

### 3. Track types & progressive new-track flow
Show all track *types* first, then ask only for type-specific info. Add
worktree-less `ask`/`investigate` and `plan` types that start instantly, with
the ability to promote to a full worktree later.
**Detail:** [`design/new-track-flow.md`](design/new-track-flow.md)

- [ ] follow-ups: Claude self-promote skill; capture a plan track's output to
      seed the promoted work prompt; per-kind configurable permission mode.

### 4. Concurrency cap & queue  *(mid prio)*
Limit how many tracks run Claude simultaneously; queue the rest and auto-start
them as slots free. Guards against melting the laptop and hitting API rate
limits. Not urgent (little parallel use at first) but will matter as usage grows.
Pairs with the token/cost work — optionally pause spawning near a spend/rate
ceiling. A Race track (topic 3) consumes 3 slots.

- [ ] config: `max_running` (+ optional spend/rate ceiling).
- [ ] queue + auto-start as slots free; show queued state in dashboard.

### 5. Distribution: install script, versioning & automated releases
Tracks is build-from-source only today (`make build` / `go build`, run `./tracks`
from the repo). To install it standalone / share it with others, it needs a real
distribution story. **Directly relevant to the daemon-staleness work** (PRs #18,
#21): `main.Version` is hardcoded `0.1.0` and only ldflags override it, so without
proper versioning the CLI can't tell a freshly-updated install from the running
daemon — `ensureDaemonUp`'s version-mismatch restart never fires. PR #21 added an
mtime fallback that covers same-path installs (the install script overwrites the
same binary), but a real version is the clean, portable signal — and it's what
covers the PATH-installed case where the mtime check is deliberately skipped. So
proper releases *finish* the "update Tracks without manually bouncing the daemon"
fix for installed users, who will otherwise hit the exact same stale-daemon
problem on every upgrade.

- [x] **Install script** — `scripts/install.sh` (curl | bash) detects OS/arch,
      downloads the matching release asset, and installs to `~/.tracks` with
      PATH injection (mirrors the stac-man installer). `scripts/uninstall.sh`
      does the reverse. *No checksum yet* — stac-man doesn't verify one either;
      left as a possible follow-up.
- [x] **Versioning** — semver + git tags via release-please; tagged builds stamp
      `main.Version`/`BuildTime` through the existing `LDFLAGS`, and the
      `x-release-please-version` marker in `main.go` is bumped by the config's
      `extra-files` generic replacer.
- [x] **release-please** — `.github/workflows/release-please.yml` +
      `release-please-config.json` / `.release-please-manifest.json` shipped
      (plus a `ci.yml` lint/build/test workflow). Automates changelog +
      version-bump PR, tag on merge, and build-and-attach of the cross-compiled
      binaries to the GitHub release. **Requires a `PAT_TOKEN` repo secret** for
      the first release to publish assets.
- [ ] **Update safety (closes the daemon loop)** — on upgrade the daemon must
      actually restart onto the new binary. A real version bump makes
      `ensureDaemonUp` restart it on the next `tracks` run for *installed* users,
      the same way PR #21 does for local `go build`. Consider a `tracks upgrade`
      that re-runs the installer and bounces the daemon; make sure the one-time
      "old daemon can't restart itself" caveat is handled by the installer.

---

## Bugs  🐛

Active defects to fix before cutting the next release.

### Bug 1 — Dev server / proxy can only be started with `tracks` installed  ✅ FIXED

**Root cause**: No install script and no automated release process. Users must build from source.

**Fix (shipped)**: `scripts/install.sh` (detect OS/arch, download from GitHub
releases, install to `~/.tracks` + PATH) and `scripts/uninstall.sh`.
`.github/workflows/release-please.yml` + `release-please-config.json` +
`.release-please-manifest.json` automate semver tagging, changelog, and
cross-compiled release assets; `.github/workflows/ci.yml` adds lint/build/test.
Mirrors the stac-man approach; `main`-only, no develop branch. *First release
needs a `PAT_TOKEN` repo secret to publish assets.*

> **Bugs 2–6 fixed** (`b86548a`) and removed from this list: tmux pane not
> selected (dropped `-d`), missing proxy overlay-menu entry, eager `pnpm install`
> at creation (now deferred to first `tracks up`), and the empty dashboard SVC
> column. Bug 3 (proxy `<prefix>+p` clobbering `previous-window`) was resolved by
> design — there is no standalone proxy keybinding; the overlay-menu entry is the
> surface.

---

## Reliability & robustness  ⭐

> What most erodes trust in Tracks day-to-day: healthy tracks getting errored,
> worktrees (and uncommitted work) vanishing, status/logs that lie. This is a
> standing section — add reliability defects here as they bite, fix, then
> delete. Ordered by how often it actually hurts; **A–C first.**

- [ ] **A. ⭐ Never delete work out from under a live session.** *(Bit us: a
      running track's worktree was GC'd mid-session, losing uncommitted work.)*
      `RemoveWorktree` (`internal/git/primary.go`) and GC (`recovery.go`
      `gcOrphanedWorktrees`, `cmd/gc.go`) use `git worktree remove --force` +
      `os.RemoveAll` unconditionally. Guard removal: refuse — or **quarantine**
      to `worktrees/_recovered/<id>/` — any worktree with uncommitted changes or
      unpushed commits, instead of `rm -rf`. End/Kill should detect dirty/
      unpushed state and say so in the confirm (pairs with the "Confirm before
      End / Kill" small task, but with a real safety check).

- [ ] **B. ⭐ Survive a daemon restart without erroring healthy tracks.** *(A
      config reload / daemon bounce currently marks every running track
      Errored.)* `reconcileOnStartup` (`internal/daemon/recovery.go`)
      unconditionally marks every non-terminal track Errored "because we can't
      re-supervise across restarts" — even though it already does a `kill -0`
      liveness check and Claude is usually still alive in its tmux pane. Instead:
      if the PID is alive and the pane is live, **re-adopt** the track as running
      (re-attach the supervisor: resume pane polling + process-group tracking);
      only mark Errored when the process is genuinely gone.
      *Partial:* `pr` tracks are already re-adopted on restart (`07b082d`,
      `recovery.go`) via a review-only supervisor; drafts are now skipped too.
      Every other non-terminal track is still marked Errored — this item is the
      general case: extend the same liveness-based re-adoption to `running` tracks.

- [ ] **C. Tell the truth about status & logs.**
  - `Track.LogPath` points at `<state_dir>/logs/<id>.jsonl`, which
    `internal/usage/usage.go` documents is **never written** (Claude runs
    interactively, not `--print`). Drop the field, or point it at the real
    transcript (`~/.claude/projects/<cwd>/<session>.jsonl`, already computed by
    the usage package) so `tracks doctor` / "show me the log" actually work.
  - Distinguish clean exit vs crash vs **auth-expiry** ("Please run /login") vs
    killed, as a terminal status + exit reason. (A roadmap track auth-expired
    mid-run but showed `done`.)

- [ ] **D. Encode the lifecycle + fault tests (stop the whack-a-mole).** *(The
      git log is a row of one-off lifecycle fixes: index.lock stealing,
      process-group kill, Stop-hook ENOENT, blank release popup, false "PR
      opened", daemon ctx-cancel wedge.)* Define the track lifecycle as an
      explicit state machine with idempotent terminal transitions, and add a
      fault-injection test suite (daemon killed mid-op, tmux pane died, git op
      fails, stale socket) so these classes stop regressing.

- [ ] **E. Self-healing `tracks doctor`.** *(Gives the `tracks doctor` small task
      teeth.)* Detect state↔reality drift — PID marked running but dead, worktree
      missing on disk, orphan dirs, stale socket — and offer one-key repair,
      reusing the reconciliation logic already in `recovery.go`.

- [ ] **F. Git concurrency discipline.** The index.lock fix (`4e7e431`) was a
      symptom. General cure: serialize / retry-with-backoff around git
      invocations in a worktree Claude is also using, so tracks and Claude don't
      collide on `.git`.

Self-contained improvements/fixes. Add new ones here; tick + delete when done.

- [ ] **"Save as draft" on Esc-cancel.** The Esc-confirm itself shipped
      (`caea558`: "Discard this track? Keep editing / Discard", fields
      repopulated from their bound pointers). Still open is the stretch third
      option — persist the partially-filled form as a draft. This can now reuse
      the draft model shipped for failed creations (`StatusDraft` + `Track.Draft`
      + `tracks launch`); the remaining work is capturing the form's params into
      a draft on cancel instead of only on a failed creation. See parking lot.

- [ ] **Confirm before End / Kill.** Both are mildly destructive (stop Claude +
      remove the worktree) — add a "Stop track <slug>? Yes/No" confirm to both
      End (`MethodDone`) and Kill (`MethodKill`). Note: neither deletes the
      branch (your commits survive), so the confirm wording should say "stops
      Claude and removes the worktree", not "deletes your work". Kill's extra
      bite is SIGKILL-immediately vs End's 5s SIGTERM grace.

- [ ] **Disk-usage visibility.** GC + startup reconciliation already exist
      (`cmd/gc.go`, `internal/daemon/recovery.go`). Missing: a disk-usage column
      / total in the dashboard + a warning threshold (worktrees + node_modules
      pile up fast on big repos). Optionally auto-suggest `tracks gc`.

- [ ] **CI / checks status in the dashboard.** PR state is already polled; also
      surface GitHub check status (pending/pass/fail) next to the PR URL so the
      dashboard is the one place to watch.

- [ ] **`tracks doctor`** — preflight: tmux / claude / git on PATH, config
      sanity, socket health, (for mobile) Xcode + simulator availability.

- [ ] **Ergonomics:** attach-by-slug + fuzzy attach (don't need the long ID);
      shell completions (zsh/bash/fish) incl. completing track slugs / repo
      names.

- [ ] **Jira ticket → task prompt (quality improvement).** When the prompt
      mentions a ticket (e.g. ABC-123), fetch its description + acceptance
      criteria via the Atlassian MCP and feed them to Claude. Reuses the
      existing Jira auth/plumbing (already used for transition/assign). Hybrid:
      inject the snapshot for guaranteed grounding *and* keep the MCP available
      in-track for on-demand follow-up (comments, linked issues). Biggest win on
      terse prompts (`fix ABC-123`).
  - **Keep the displayed prompt short.** Don't overwrite the user's short
    `TaskPrompt` with the full ticket body — build the enriched prompt only at
    spawn time (passed to Claude), leaving `TaskPrompt` as the short label the
    dashboard shows. Otherwise the dashboard details panel
    (`internal/tui/dashboard/info.go:118-120`, which wraps the full prompt)
    breaks on long tickets. If injection must persist into `TaskPrompt`,
    truncate to the first line / N chars in `info.go` instead.

---

## Ideas / parking lot

Raw, uncommitted thoughts — promote to a section above when they firm up.

- **Draft tracks.** A track you've started configuring (or fully described) but
  haven't launched — persisted so you can resume/edit/launch later. The core
  model now exists (shipped for failed creations: `StatusDraft`, `Track.Draft`,
  `tracks launch`, dashboard `L`/`x`). Remaining: let the user create a draft
  *deliberately* (not only after a failure) — the "Save as draft" option on
  Esc-cancel (small tasks) and the worktree-less types in topic 3 (a draft is a
  not-yet-started track with no worktree) are the two entry points to wire up.

- **Stacked tracks.** A track that branches off *another track's* branch instead
  of the repo base, so dependent work parallelizes (mirrors the `stac-man`
  stacked-PR workflow). Pick a "parent track / branch" at creation.

- **Race track** *(future)* — a track type that runs 3 agents in parallel on the
  same prompt for very tricky problems; pick the best result, discard the rest.
  Detail in [`design/new-track-flow.md`](design/new-track-flow.md). Uses 3
  concurrency slots (topic 4).

- **Cross-track conflict detection** *(low prio)* — warn when two live tracks
  edit overlapping files in the same repo (collision risk at merge time).
  Compute from each worktree's changed-file set.

- ~~**Stable-port proxy / dev-server switchboard**~~ — **Shipped** in this PR.
  `proxy_port:` on a service config; `tracks proxy` / `tracks proxy switch`;
  auto-wired on `tracks up/down`. WebSocket-friendly (FlushInterval -1).
  State persisted in `proxy.json` — survives daemon restart.

---

## Recently shipped

Move completed items here with a date, then delete once the dust settles.

- **2026-07-11 — Save a failed track creation as a draft.** A creation that
  fails mid-provisioning (e.g. a git fetch/clone rejected for an expired GitHub
  token) now captures the entered params on the errored track; the user chooses
  **Save as draft** or **Dismiss** at failure time. New non-terminal
  `StatusDraft` + `Track.Draft`, `save_draft` / `launch` daemon methods, a
  GitHub auth/permission hint on the failure message, dashboard `L` (launch) /
  `x` (dismiss) + a DRAFT detail block. Also fixed a data-loss bug where the
  daemon server tests ran startup GC against the real `~/.local/state/tracks`
  and deleted live worktrees. `internal/state`, `internal/daemon`,
  `internal/tui/{newtrack,dashboard}`, `cmd/{menu,new}`. (`c76aec9`, `92a7710`)
- **2026-07-06/11 — Resume finished tracks + error visibility.** Follow-up
  prompt to a finished track reusing its worktree + Claude session (dashboard
  `R`); failed creations surfaced on the dashboard with a stored reason;
  per-repo settings submenu + service CRUD; per-repo "open PRs as draft".
  (`ac7ed74`, `b68bdfd`, `2edbdeb`, `8622d01`, `55e73d4`)
- **2026-07-06 — Bugs 2–6 fixed** (see Bugs section). tmux pane selection,
  proxy overlay-menu entry, lazy deps install, dashboard SVC column; Bug 3
  resolved by design. (`b86548a`)
- **2026-07-01 — PR / in-review state** (was Reliability G). Non-terminal
  `StatusPR`; a track with an open PR stays in review (worktree + supervisor
  kept) instead of flipping to `done` on Claude exit; the PR watcher drives the
  final Done on merge/close and keeps refreshing usage; `pr` tracks re-adopted
  across daemon restart. (`07b082d`, `ddfe201`, `d2958d0`)
- **2026-06-29 — apfs-clone provisioning cache** (Topic 0 v1). COW-clone the
  primary's `node_modules` before the deps install; plain-copy fallback off
  APFS. (`7d91e10`)
- **2026-06-29 — Confirm before discarding a new-track form on Esc.** "Discard
  this track? Keep editing / Discard", fields repopulated from bound pointers.
  (`caea558`)
- **2026-07-02 — Dev-server control surface + stable-port proxy** (Topic 1 v1a).
  `tracks up/down/services/url` CLI; tmux log-viewer panes (right 35% column);
  `service_ready` notification; stable-port reverse proxy (`proxy_port:` in
  service config, `tracks proxy` / `tracks proxy switch`); auto-switch on up/down;
  WebSocket-friendly (HMR works through proxy); `proxy.json` persistence.
  `internal/proxy`, `internal/daemon/service_handlers.go`,
  `internal/daemon/proxy_handlers.go`, `cmd/services.go`. (`3417537`, this PR)

- **2026-06-24 — Worktree provisioning v1** (Topic 0). `provision.deps_cmd` +
  gitignored `copy_ignored`/`copy_mode`, run after worktree create / add-repo;
  Settings TUI form extended. `internal/provision`, `internal/daemon/handlers.go`.
  (`ec56523`)
- **2026-06-25 — Track types & progressive new-track flow v1** (Topic 3).
  Progressive type picker, worktree-less `ask`/`plan` (read-only plan mode),
  `tracks promote`, add-repo to a running track, `Kind` on `Track` (+ schema v2
  migration) + dashboard KIND badge. (`0f95649`, `f99cf3d`)
- **2026-06-25 — Token / cost + time summary per track.** Transcript-based
  token/cost/runtime (`claude --session-id`); COST column, detail-panel
  breakdown, and a `… tok / $… / …m` tail on the done notification.
  `internal/usage`, supervisor, dashboard. (`a0aa3db`)
