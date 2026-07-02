# Design & Plan: Dev servers (and autonomous testing) inside a track

**Status:** Living plan — edit freely as ideas/time allow.
**Last updated:** 2026-06-24

This is a working document, not a spec. Sections marked *(decided)* reflect
choices made so far; *(open)* sections are still up for grabs. Update it
whenever the design shifts.

---

## 1. Problem

When Claude makes changes on a branch inside a track worktree, there's no way
to *run* those changes without releasing/merging them. We want a track to be
able to spin up the relevant dev servers (Ledger Live Desktop, live apps like
swap-live-app, and later Ledger Live Mobile) so the work can be exercised
in-place — first by the human, eventually by Claude itself.

Two hard constraints:

1. **No collision** with dev servers the user runs manually in Cursor / Ghostty
   (default ports — Metro 8081, etc.).
2. **No collision between concurrent tracks** running the same services.

---

## 2. Guiding principle  *(decided)*

> The binary stays **mechanism, never policy.** It learns to run named
> processes, allocate ports, run hooks, and (later) hand off to autonomous
> drivers. It never learns anything Ledger-specific — no "metro", no
> "manifest", no `~/ledger`, no repo names.

Ledger knowledge lives in `config.yaml` + hook scripts, versioned and shared as
a `tracks-ledger-setup` starter kit. This dissolves the "Ledger-specific vs.
shareable" tension: deep automation in config/hooks, generic tool stays
open-sourceable.

---

## 3. Architecture overview

Current state (for reference):
- One long-running process per track: Claude, in a tmux pane.
- Supervisor polls every 2s: PID liveness, sentinel file, `tmux capture-pane`
  for "waiting", diff shortstat for `Changes`, scans pane for `TRACKS_PR_URL=`.
- `notify` (macOS notifications + bell) on status transitions.
- Dashboard reads `state.json`.

### Generic primitives to add to the binary

1. **Named services** per repo (config) — each a background process in its own
   tmux pane, supervised by the (generalized) supervisor so they die with the
   track. No orphaned bundlers.
2. **Deterministic port allocator** — per-track port block derived from the
   unique track ID (hash → base, or incrementing slot in `state.json`). Block
   starts high (**20000+**) so manual default-port servers are never touched.
   Persist allocated ports on the `Track`.
3. **Readiness probe** — a service is "up" when a condition holds, not when the
   PID spawns. Config: `ready: { port: <n> }` or `ready: { log_regex: "..." }`.
   Everything downstream (handoff notification, autonomous driving, dependency
   ordering) depends on this.
4. **Hook lifecycle** — `pre_start` / `post_start` / `pre_stop`, per service
   and/or per track. This is where Ledger-specific wiring lives (e.g. patch a
   live-app manifest URL with the allocated port).
5. **Templating** — for cmd/env/hooks: `{{.Port "name"}}`, track id, worktree
   paths, etc.
6. **Dependency ordering** — `depends_on: [lld]`; honor via ordered
   wait-for-ready. Keep it a simple ordered list, not a DAG, until proven
   necessary.

### Config shape (proposed)

```yaml
repos:
  - name: ledger-live
    path: ~/ledger/ledger-live
    base: develop
    services:
      - name: lld                         # Ledger Live Desktop (Electron)
        cmd: "pnpm dev:lld"
        env:
          PORT: '{{.Port "lld"}}'
        ready:
          log_regex: "compiled successfully"
      - name: live-app
        cmd: 'pnpm --filter swap dev --port {{.Port "live-app"}}'
        env:
          PORT: '{{.Port "live-app"}}'
        ready:
          port: '{{.Port "live-app"}}'
        post_start:
          # Ledger-specific manifest patching lives here, NOT in the binary.
          - 'scripts/patch-manifest.sh {{.Port "live-app"}}'
        depends_on: [lld]
      - name: metro                       # v1b — mobile
        cmd: 'pnpm mobile:start --port {{.Port "metro"}}'
        env:
          RCT_METRO_PORT: '{{.Port "metro"}}'
        ready:
          port: '{{.Port "metro"}}'
```

### Window layout  *(decided — headless process + viewer pane)*

Services run **headless**: the daemon owns each process directly (see Teardown)
and streams its stdout+stderr to a per-service log file. Visibility is a
separate, purely cosmetic concern — when a track has running services, split
the track's tmux window and `tail` each service's log live. The panes *view*
the logs; they never *own* the processes.

- **Left ~65% width:** Claude's pane (the main interactive pane, as today).
- **Right ~35% width:** a column of **viewer panes**, one per running service,
  each just `tail -f`-ing that service's log file, so LLD / live-app / Metro
  logs are visible side-by-side without leaving the window or spawning a
  separate tmux session.

Notes / implementation:
- Only split when at least one service is running; a track with no services
  keeps the full-width Claude pane (don't waste space).
- Create the right column on first service start (`tmux split-window -h -p 35`
  running `tail -f <logfile>`), then stack subsequent services into it
  (`split-window -v` targeting the right column). Re-balance with
  `select-layout` / explicit sizing so the 65/35 split holds as panes are added.
- Pane title = service name + assigned port, so it's obvious which is which.
- On service stop, close its viewer pane; when the last service stops, collapse
  the right column back to full-width Claude. Closing a viewer pane never
  touches the process — teardown is always the group-kill below.
- Lives in `internal/tmux` alongside the existing window/pane management.

> **Why viewer panes, not run-in-pane:** keeping the daemon as the process owner
> preserves the group-kill teardown and log-regex readiness the engine already
> ships (below); the pane is a dumb `tail`, so it can't affect lifecycle. The
> alternative (launch the server *inside* the pane, tmux owns it) was considered
> and rejected — it would move teardown/readiness onto the pane pid and
> `capture-pane`, reworking code that already exists and is tested.

**Status: v1a complete.** Services engine (PRs #14–#17) + control surface,
viewer panes, stable-port proxy, and `service_ready` notification (this PR).
`tracks up/down/services/url/proxy` all work end-to-end.

### Teardown — services must die with the track  *(decided)*

Services must be torn down whenever the track stops, on **all** paths:
- **End** (`MethodDone`) — graceful: SIGTERM each service (short grace), then
  remove worktree.
- **Kill** (`MethodKill`) — SIGKILL each service immediately.
- **Quit session** / daemon shutdown (`Server.Stop`) — tear down services across
  **all** tracks, not just Claude.
- Startup reconciliation (`recovery.go`) — GC any service processes orphaned by
  a daemon crash.

**The gotcha: kill the process *tree*, not just the pane.** Node dev servers
fork children (webpack/metro workers; Metro also spawns watchman). Killing the
tmux pane's shell can orphan those children, leaving servers bound to the
allocated ports. So:
- Start each service in its **own process group** (e.g. `setsid` / `Setpgid`),
  record the **PGID** on the service's state.
- On teardown, signal the **whole process group** (`kill -- -<pgid>`), SIGTERM
  then SIGKILL, before/instead of just closing the pane.
- Closing the tmux pane is cosmetic cleanup — the authoritative kill is the
  process-group signal.
- After teardown, optionally verify the allocated ports are free (belt-and-
  suspenders against stragglers like watchman).

This is what backs the "no orphaned bundlers" promise in §3; reuse and extend
the existing supervisor teardown (`supervisor.go` Kill, `handlers.go`
shutdown) so it iterates services too.

### Lifecycle  *(open — see §6)*

Leaning toward **lazy-start**: services are declared but dormant; ports are
allocated at track creation (just arithmetic) but nothing runs until
`tracks up <service>` (CLI / MCP tool). Avoids frying the laptop when several
tracks exist, and sidesteps the resource-ceiling question.

---

## 4. The cross-port wiring problem  *(Ledger-specific → hooks)*

Naive port-swapping breaks because the *client* side must know the port too:

- **Live apps** — the dev server port is the easy half. Ledger Live loads a live
  app from a **manifest whose URL points at `localhost:<port>`**. Changing the
  port means regenerating/patching the local manifest the Discover/dev panel
  loads → handled by a `post_start` hook.
- **Mobile / Metro** — `--port` on the server isn't enough; the app must bind to
  the same port (`RCT_METRO_PORT` / in-app "Debug server host & port") → handled
  via service `env`.
- **LLD renderer** — webpack dev server port via env; least painful.

All expressed as `env` + hooks, none in the binary.

---

## 5. Phased plan

### Phase v1a — LLD + live apps, human drives  *(first build)*

Goal: from a track, start the necessary dev servers, wait for readiness, notify
the dev that manual testing can begin (with URLs). Human drives the browser.

Build:
- [x] Config: `services` array on repo entries (cmd, env, ready, hooks,
      depends_on). Parse + validate. *(PR #14)*
- [x] Port allocator: per-track block (20000+), persisted on `Track`,
      reproducible. Templating resolver for `{{.Port "name"}}`. *(PR #14)*
- [x] Generalize supervisor: track N **daemon-owned** service processes per
      track (PID + PGID each), not just Claude. Services die on `done`/`kill`.
      *(PRs #15, #17)*
- [x] Readiness probe: port-listen and log-regex variants. *(PR #16)*
- [x] Hook runner: `pre_start` / `post_start` / `pre_stop`. *(PR #16)*
- [x] tmux **viewer pane** per service: `tail -f` the service log in a pane in
      the right column of the track window (see Window layout). Cosmetic only —
      the daemon still owns the process.
- [x] CLI + MCP control surface: `tracks up <service>`, `tracks down <service>`,
      `tracks services` (status + ports), `tracks url <service>`.
- [x] notify: `service_ready` event with URL (stable port shown when proxy active).
- [ ] Dashboard: services column (name, port, status) — still outstanding.

Mostly rides existing rails (supervisor poll loop, notify, dashboard,
`TRACKS_PR_URL` scan pattern).

### Phase v1b — mobile, manual  *(fast follow)*

Goal: boot Metro + launch the simulator from a track, hand off to the human.
Still human-driven — skips autonomous RN driving.

Build:
- [ ] Metro as a service (port via `RCT_METRO_PORT`).
- [ ] Simulator launch as a service/hook (`pnpm mobile:ios` / `xcrun simctl`).
- [ ] Reuse readiness + handoff notification from v1a.

### Phase v1c — autonomous mobile smoke-test via Argent  *(pulled forward — high value, low effort)*

Insight: **Argent is the automation engine; tracks just provides isolation +
wiring.** Argent (MCP server + skills) lets the in-track Claude boot the
simulator, build/install/launch the app, drive UI (tap/swipe/type/gesture),
get a screenshot after each action, inspect the RN component tree, read logs,
watch network, and profile. So tracks does NOT build a mobile automation engine.

This can land largely **independent of the generic services engine** (§5 v1a),
because Argent handles boot/build/launch itself. The only tracks-side work is
isolation + MCP registration + a task-suffix instruction.

What tracks must do:
- [ ] **Register Argent's MCP for the in-track Claude** when mobile testing is
      enabled for a repo — inject `--mcp-config` or write `.mcp.json` into the
      worktree. (Argent writes project `.mcp.json` or `~/.claude.json` on
      `argent init`.)
- [ ] **Isolate from the user's manual setup** (the one real collision risk):
  - Metro port from the track's port block → `RCT_METRO_PORT` / `--port`.
  - A **dedicated simulator device per track** (unique name/UDID, e.g.
    `tracks-<id>`) so two booted sims + two apps + two Metros don't fight.
- [ ] **Task-suffix instruction**: after implementing, use Argent to launch and
      smoke-test the change, capture screenshots/findings, then hand off (or, in
      full v2, gate the PR on it).

The fast path that makes this "easy":
- **JS/TS-only change (common case):** reuse an already-installed app build,
  point Metro at the *worktree's* JS on the allocated port, reload. Fast — this
  is Argent's "iterate" loop. No native rebuild.
- **Native change (rarer):** full rebuild in the worktree (pods + native
  compile) — slow; document as opt-in / expect latency.

Open unknowns to spike (each ~minutes):
- Does Argent let you target a specific simulator **UDID** (for per-track
  isolation)?
- Does Argent let you set/inherit the **Metro port**, or does it assume 8081?
- Does building/iterating work cleanly from a **worktree** checkout of
  ledger-live mobile (vs the primary)?
- iOS only today (Android emulator "in the works") — fine for MVP.

Prereqs: macOS + Xcode, Node 20.11+. Install: `npx @swmansion/argent init`.

### Phase v2 — full automation: Claude operates the app  *(future)*

Goal: autonomous verify loop. Claude starts servers → drives the running app →
asserts → reviews (`tracks-reviewer`) → opens PR. Dashboard gains a "verified"
signal next to `Changes`. Bolts onto the same services/ports/readiness
primitives — autonomous drivers are just another consumer of "server up on
port X".

**Desktop (Electron/LLD) → Playwright.**
- Playwright has first-class Electron support (`_electron.launch()` drives the
  renderer like a normal page). Official **Playwright MCP** server → Claude
  clicks/types/asserts via MCP.
- Most tractable (Electron == Chromium).
- Note: autonomous mode launches LLD *via Playwright*, so the "service" shape
  differs from v1a's `pnpm dev:lld`. Consider a `mode: manual|driven` on the
  service, or a separate service definition for driven runs.
- Build: [ ] Playwright(-MCP) wired as a per-track capability pointed at the
  allocated LLD port; [ ] a verify-loop convention Claude follows (start →
  drive → assert → report); [ ] surface results to dashboard.

**Mobile → Argent (primary), Maestro/Appium as fallbacks.**
- **Argent** (https://argent.swmansion.com/, Software Mansion) is purpose-built
  for this: "an agentic toolkit to enable AI agents to autonomously control,
  debug, and profile iOS and Android applications" — **a set of skills + an MCP
  server** that connect AI harnesses directly to the iOS Simulator and Android
  emulator. The agent taps/swipes/types/navigates via accessibility-tree
  coordinates, runs multi-step sequences, reads console logs, inspects React
  component trees, monitors JS+native network requests, and profiles
  (React + native, UI hangs, render cascades, memory leaks).
  - Explicitly supports **Claude Code** ("any agent that can run shell
    commands"). Install: `npx @swmansion/argent init`. Free on npm.
  - This is the most agent-native fit — built for an agent driving a *live* app,
    vs. authoring test scripts. Likely the v2 mobile driver of choice.
- **Maestro** (YAML flows) / **Appium** — fallbacks/alternatives if we want
  scripted, repeatable flows rather than live agentic driving.
- Related but distinct: **Radon IDE** (https://radon.swmansion.com/) embeds the
  simulator in VSCode/Cursor — human-dev-ergonomics tool, good for v1b manual
  testing. Its "Radon AI" also ships an MCP server (reload, logs, screenshots).
- Build: [ ] Argent installed/available in the track; [ ] its MCP server wired
  to the in-Claude verify loop against the booted simulator + Metro port;
  [ ] results to dashboard.

**Cross-cutting v2:**
- [ ] MCP surface for Claude to start/stop/drive services and read logs.
- [ ] Verify-loop integration with the existing `tracks-reviewer` gate (verified
      ✓ becomes part of the pre-PR checklist).
- [ ] Resource ceiling: cap concurrent driven tracks (LLD + Playwright + Metro +
      sim is heavy).

---

## 6. Shareability work  *(parallel)*

Current blocker to sharing with teammates: config encodes the local machine
(`~/ledger/...`).

- [ ] `${VAR}` / env expansion + relative paths in config.
- [ ] Checked-in template config + hook scripts as a `tracks-ledger-setup`
      starter repo. Teammates clone, set `LEDGER_ROOT`, run.

---

## 7. Open questions / parking lot

- **Lifecycle:** lazy-start (`tracks up`) vs auto-start with the track vs both?
  (Leaning lazy-start — see §3.)
- **Readiness:** default to port-listen, log-regex, or both? Per-service choice?
- **Crash handling:** auto-restart a crashed service, or just mark it red?
- **Resource ceiling:** cap concurrent service-bearing / driven tracks?
- **Dependency ordering:** ordered list (current plan) — when does a real DAG
  become necessary?
- **Service modes:** how to model manual (`pnpm dev:lld`) vs driven (launched by
  Playwright) for the same app — `mode` field, or separate service defs?
- **Claude control surface:** MCP tool vs `tracks` subcommands vs both for
  start/stop/logs/url.
- **Mobile depth:** how much simulator/device/native-rebuild wiring do we own vs
  delegate to existing scripts?
- **Handoff UX:** notification only, or also a `tracks open <service>` that
  launches the browser/app for the human?
