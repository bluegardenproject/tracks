# Backends and repo config

**Status:** proposed. Each step below is one PR; tick it off in
[docs/ROADMAP.md](../ROADMAP.md) as it ships.

## Problem

Tracks grew up on frontend work: a repo with a `pnpm install` setup, one or two
dev servers on one HTTP port each, and the proxy handing a fixed port to a
browser or a simulator. Backend repos (Scala with sbt, Python with uv or
poetry, TypeScript with Nest or Express) fit only partly:

- A service often listens on several ports: HTTP, gRPC, admin or metrics.
  Tracks gives a server one `$PORT`.
- gRPC is HTTP/2 without TLS (h2c). The proxy speaks HTTP/1.1 and WebSockets
  only, so a gRPC client can't go through an output port.
- Switching a service between tracks means moving several output ports one by
  one.
- A service needs backing services: Postgres, Kafka, Redis, usually from
  `docker compose`. Every worktree folder has the repo's name, so Compose
  gives every track the same project, and tracks share or recreate each
  other's containers.
- Local config isn't always `.env`. Scala keeps it in `application.local.conf`,
  `.jvmopts`, `.sbtopts` or `local.sbt`, Python sometimes in `.envrc`. Tracks
  copies only `.env*` into a new worktree.
- JVM details: a JDWP debugger or JMX port can be taken for the server's port
  in detect mode, and 5 s of grace is short for JVM shutdown hooks.
- Not every service serves on a port. Workers, Kafka consumers, schedulers,
  `tsc --watch` and codegen watchers never listen, so Tracks shows them as
  starting forever.
- Listening isn't ready. Play in dev mode listens at once and compiles on the
  first request; many services listen before their migrations finish.
- Setup can need a running service. Migrations and seeding (`sbt
  flyway:migrate`, `alembic upgrade head`) need Postgres up, but setup runs
  before every server.
- Toolchains are per folder. Setup and servers run through `sh -c`, so a
  repo's `.envrc` (direnv), `mise.toml`, `.java-version` or sdkman settings
  are likely not applied, and a track may get the wrong JDK or Python.
- Every new worktree builds cold: sbt compiles for minutes, `node_modules` is
  installed again.
- Parallel tracks load the machine: several sbt JVMs, Metals and Docker can
  run out of memory. Koda's config sets cpu and memory for the same reason.
- Everything about a repo is typed into the repo form, per person. A backend
  with several servers, ports and a Compose stack is a lot to type, and a team
  can't share it.
- Smaller things: `requirements.txt` counts as docs in the pre-push review
  rule (`*.txt`), prompts and the repo form give pnpm examples, server types
  know only JS tools, and the reviewer doesn't read `build.sbt`,
  `.scalafmt.conf` or `pyproject.toml` for conventions.

What already fits: setup and dev server commands are any shell line, `$PORT`
and `{{port}}` cover `sbt "run {{port}}"`, `-Dhttp.port=$PORT` and
`uvicorn --port $PORT`, readiness waits for the port to listen (so a long sbt
compile shows as starting), and teardown stops the whole process group, forked
JVMs included.

## Approach

- **Additive, frontend first.** Nothing a frontend repo has configured today
  changes. Every new feature is opt-in per repo, service or port, and defaults
  keep today's behaviour.
- **General, not per language.** Tracks knows ports, processes, files and
  Compose projects; it doesn't learn sbt or uv. Language shows up only in
  labels, examples and the commands a repo declares.
- **The repo says what it needs.** A checked-in `.tracks/config.yaml` carries
  what a team shares. Each repo's settings come either from that file or from
  the form, never a mix; a few personal fields stay editable either way. Every
  repo setting below gets its key in the file.
- **Field test first.** Before the bigger steps, run one real Scala service
  and one Python service on today's Tracks and write down what breaks.
- **Documented where it's used.** Every key and feature has a short topic built
  into Tracks, readable by the user in the UI and by agents from the command
  line.

## Naming

Dev servers become **services**: a worker, a consumer or a Compose stack is
no dev server. Service reads right for a frontend dev server, a backend and a
worker alike; "process" was considered and dropped, since it's also every
program the OS runs. The word changes in the UI (the repo form's section, the
Proxy tab's pickers), in the file (`services:`), in prompts and in the docs.
The CLI keeps `tracks up`, `down`, `logs` and `url`; `tracks services` lists
them, and `tracks servers` stays as another name for it. Code and the database are
renamed too (step 2), so people and agents never translate between two names.
The Problem section above speaks of today's dev servers.

## Repo config file

`.tracks/config.yaml` at the repo's root. A folder, like `.koda/`, so prompts
or skills for the repo can join it later.

```yaml
version: 1

setup:                          # once per worktree, before services and checks
  command: sbt update flywayMigrate
  after: [deps]                 # wait for these services; they don't wait for setup
teardown: docker compose down -v  # when a track ends, before its worktree goes
copy: [".env*", "conf/application.local.conf", ".jvmopts"]
warm: ["target/", "project/target/"]  # cloned copy-on-write from the primary checkout
toolchain: auto                 # direnv or mise when the repo has their files
submodules: true

checks:                         # how to know a change is good
  format: sbt scalafmtCheckAll
  lint: sbt "scalafixAll --check"
  test: sbt test

docs: ["*.md", "docs/**"]       # what the pre-push review treats as docs only

services:
  - name: deps
    command: docker compose up
    port: { name: postgres, protocol: tcp }
    ports:
      - { name: kafka, protocol: tcp }
  - name: api
    command: sbt "run {{port}}"
    type: sbt
    port: { mode: assigned, protocol: http }
    ports:
      - { name: grpc, protocol: tcp }
    after: [deps]
    ready: { http: /health }    # or { log: "Server started" }
    stop_grace: 20s
  - name: consumer
    command: sbt "consumer/run"
    port: { mode: none }        # no port: ready while it runs
    after: [deps]

proxy:                          # presets offered on the Proxy tab
  - name: payments              # a group: several ports, one track
    ports:
      - { name: http, port: 8080, service: api }
      - { name: grpc, port: 9090, service: api, service_port: grpc }
```

A frontend repo uses the same file with fewer keys. A `proxy` entry with
`port` instead of `ports` is a simple port:

```yaml
version: 1

setup: pnpm install --frozen-lockfile
copy: [".env*"]

checks:
  lint: pnpm lint && pnpm tsc
  test: pnpm test:unit

services:
  - name: web
    command: pnpm dev --port $PORT
    dir: apps/web
    type: rspack
  - name: storybook
    command: pnpm storybook -p $PORT
    type: storybook

proxy:
  - { name: web, port: 3000, service: web }
  - { name: storybook, port: 6006, service: storybook }
```

### Who reads what

| Key | Used by |
|---|---|
| `setup`, `teardown`, `copy`, `warm`, `toolchain`, `submodules` | Track creation and ending; `toolchain` also wraps every command Tracks runs in the worktree |
| `checks` | The agent prompt: run them before calling work done. The reviewer: knows what was meant to pass. The footer later, as a button per check |
| `docs` | The pre-push review rule, instead of the fixed list |
| `services` | `tracks up`, the Services prompt paragraph |
| `proxy` | The Proxy tab's **Add from repo** |

### Source of a repo's settings

Each repo has one source, chosen in the repo form. Settings are never merged
field by field: list keys such as `services` have no clean merge, and a stale
personal override would silently win over a file that changed upstream.

| Source | Form | Where changes go |
|---|---|---|
| **Repo file** (default when the file exists) | Read-only, each section marked "from .tracks/config.yaml" | The file, through git: committed and reviewed like code |
| **Manual** (default without a file) | Editable, as today | The database |

- **Detach to manual** copies the file's current values into the form as a
  starting point. From then on the file is ignored for this user.
- **Export to file** writes the manual settings to `.tracks/config.yaml` in the
  primary checkout, for the user to commit. It's how a team starts a file: one
  person sets the repo up in the form, exports it and commits it.
- **Switch to repo file** is offered when a file appears for a repo set up
  manually. It shows what differs and never switches on its own.
- A repo on Manual whose file changed upstream gets a quiet notice with
  **Compare**, and nothing changes on its own.

### Personal fields

Some values really differ per person. These stay editable with the repo file
as source, stored in the database, each set by name and never merged into a
list, each with a **Reset**:

- the fixed port numbers of services and of `proxy` group presets;
- which services `tracks up` starts by default;
- extra environment variables, such as local secrets, which never belong in
  the file.

Anything else goes into the file, or needs **Detach to manual**. So what runs
in a track is either reviewed in git or typed by the user, never a hidden mix.

### Rules

- **Which file.** A track reads the file from its own worktree when it's
  created, so a branch that changes its services runs them. The repo form and
  the Proxy tab read the primary checkout's.
- **Trust.** The file runs commands. Tracks runs a file's commands only once
  the user has accepted that file's content (by hash), as direnv's `allow`
  does. A changed file asks again, showing what changed. A Review track on
  someone else's branch whose file differs from the accepted one asks before
  its setup or services run. Personal fields are the user's own and need no
  accepting.
- **Unknown keys** are an error shown on the repo, not ignored, so a typo
  doesn't silently drop a service. `version` lets the format change later.
- **Koda.** With no `.tracks/config.yaml`, Tracks reads `setup`, `lint` and
  `test` from `.koda/config.yaml` as a fallback, so repos set up for Koda need
  nothing more for their checks. Koda's `sandbox` key doesn't apply.
- **Personal only, never in the file:** engines, models, theme, Draft PRs.

## Proxy

### Simple ports and groups

- A **simple port** is today's output port with an optional name: `3000 ·
  web`. Its input is one running service.
- A **group** is a named set of output ports whose input is a **track**. Each
  port in the group binds to a service of that track, and a named port of that
  service for extra ports. Choosing a track rebinds every port at once.
- A port whose binding has nothing running in the chosen track answers 503 in
  HTTP mode, or closes the connection in TCP mode. It never falls back to
  another track.
- Storage, the daemon and `tracks proxies` see a simple port as a group of one
  whose input is a service. One code path.

### Modes

- **HTTP** (default): today's reverse proxy, Host kept, WebSockets and SSE
  passed.
- **TCP**: bytes forwarded both ways. Covers gRPC and anything else that
  isn't HTTP/1. A new input applies to new connections.
- Later, HTTP mode may route by path on one port (`/api/*` to a backend, the
  rest to the frontend), so a full-stack track needs no CORS setup.
- Out of scope: TLS, load balancing, auth, header rewriting.

### Proxy tab

Simple ports and groups share one list, in the order they were added:

```
╭─ Ports ──────────────────────────────────────────────────────────────────────╮
│ ╭─ web ────────────────────────────────────────────────────── forwarding ─╮  │
│ │ [ 3000 ]  [ web       ]  ‹ HTTP ›  →  [ web · rspack · track-x ▾ ]    × │  │
│ ╰─────────────────────────────────────────────────────────────────────────╯  │
│ ╭─ storybook ────────────────────────────────────────────── no service ⚠ ─╮  │
│ │ [ 6006 ]  [ storybook ]  ‹ HTTP ›  →  [ storybook · track-y   ▾ ]     × │  │
│ ╰─────────────────────────────────────────────────────────────────────────╯  │
│ ╭─ payments · group ───────────────── track [ track-x · LIVE-123 ▾ ]  × ──╮  │
│ │  Port      Name       Mode      Binds to          State                 │  │
│ │  [ 8080 ]  [ http   ]  ‹ HTTP ›  [ api        ▾ ]  ready  :20021      × │  │
│ │  [ 9090 ]  [ grpc   ]  ‹ TCP  ›  [ api · grpc ▾ ]  ready  :20022      × │  │
│ │  [ 5432 ]  [ db     ]  ‹ TCP  ›  [ deps       ▾ ]  not running        × │  │
│ │  [ 9095 ]  [ admin  ]  ‹ HTTP ›  [ api · admin▾ ]  blocked by java    × │  │
│ │                                                    (pid 4242)           │  │
│ │  [ Add port ]                                                           │  │
│ ╰─────────────────────────────────────────────────────────────────────────╯  │
│                                                                              │
│  [ Add port ]   [ Add group ]   [ Add from repo ]                            │
╰──────────────────────────────────────────────────────────────────────────────╯
```

- **Simple port:** a frame titled with its name, or its port number when it has
  none. Left to right: port, name (optional), mode, input (a running service,
  as today), remove. The frame's top right shows its state: forwarding, no
  service ⚠, or blocked by a process.
- **Group:** a frame titled with its name and `group`. Its top right holds the
  input (a track) and remove. Each row: port, name (required), mode, binding,
  state, remove. State is per row, since one port of a track may run while
  another doesn't.
- A binding is a picker of the services and named ports the group's repos
  define, not of running ones, since the track comes from the group's input.
- The tab title's green `●` counts a group as forwarding while any of its ports
  forwards to a running service.

The group's track picker lists the open tracks that have any of the group's
services, with how many of its bindings are ready. The first entry is **none**:

```
            ╭─ Track for payments ─────────────────╮
            │   none                               │
            │ ▸ track-x   LIVE-123    3 of 4 ready │
            │   track-y   SWAP-99     not running  │
            ╰──────────────────────────────────────╯
```

**Add from repo** lists the `proxy` presets of the repos' config files, simple
ports and groups alike, and adds the chosen one as is. A preset already on the
tab is marked and can't be added twice. Port numbers are suggestions: a clash
is shown under the port as today, and the port number is a personal field.

```
            ╭─ Add from repo ──────────────────────────────╮
            │ ▸ swap-live-app   web        3000            │
            │   swap-live-app   storybook  6006   added    │
            │   payments-api    payments   8080 9090 5432  │
            ╰──────────────────────────────────────────────╯
```

## Repo settings

A service's frame gains, opt-in:

| Field | Notes |
|---|---|
| Port mode **none** | Next to assigned, fixed and detect. For workers, consumers and watchers: ready while the service runs, never offered in the proxy, no `$PORT` |
| Protocol of the main port | HTTP (default) or TCP. A group port bound to it starts in that mode |
| Extra ports | An **Add port** button in the service's frame. Each has a name and a protocol, is always assigned, and is passed as `$PORT_<NAME>` and `{{port.<name>}}` |
| Starts after | Under **More options**: services to wait for until they're ready: `api` in the same repo, `payments-api/api` in another repo of the track. A service named in another repo that the track doesn't have is skipped and shown as a warning |
| Log file | Under **More options**: also write the output to a file, for `tracks logs` beyond the pane's scrollback |
| Ready check | Under **More options**: an HTTP path that must answer 2xx or 3xx on the main port, or a pattern the pane's output must print. Without one, ready means listening, as today |
| Stop grace | Under **More options**: seconds before SIGKILL, default 5 |

```
╭─ Service 1 · api ────────────────────────────────────────╮
│ Name       [ api                              ]          │
│ Command    [ sbt "run {{port}}"               ]          │
│ Port       ‹ assigned ›   Protocol ‹ HTTP ›              │
│ ╭─ Port · grpc ─────────────────────────────╮            │
│ │ Name [ grpc ]   Protocol ‹ TCP ›        × │            │
│ ╰───────────────────────────────────────────╯            │
│ [ Add port ]                                             │
│ ▸ More options   (starts after, ready check, stop grace) │
│ [ Remove service ]                                       │
╰──────────────────────────────────────────────────────────╯
```

The repo gains:

| Field | Notes |
|---|---|
| Copy into worktrees | Patterns of ignored files, default `.env*` |
| Warm folders | Ignored folders (`target/`, `node_modules`) cloned from the primary checkout with copy-on-write: APFS clonefile on macOS, reflink on Linux, skipped where neither works. Never for `.venv`, which stores absolute paths |
| Toolchain | **auto** (default with a file present): commands run through `direnv exec` when the repo has `.envrc`, `mise exec` when it has `mise.toml` or `.tool-versions`. **off** runs them as today |
| Submodules | A toggle |
| Setup waits for | Under Setup: services that start before setup and that setup waits for until they're ready, such as a Postgres for migrations |
| Teardown | A section next to Setup |

A Compose stack is a service like any other, with ports such as `postgres`
and `kafka`; its compose file maps `${PORT:-5432}:5432`.

The track's port block grows from 10 to 20 ports (500 tracks still fit in
20000–29999); blocks already claimed keep 10 until their track ends. Services in
one repo read another repo's ports in the same track as
`$TRACKS_PORT_<REPO>_<SERVER>[_<NAME>]`, for a frontend and its backend.

## Agents and services

Agents start, stop and proxy services when the user asks, in the user's words.

### What the agent understands

The Services paragraph of the prompt and the `services` docs topic map
everyday requests to commands, so the user never has to know them:

| The user says, for example | The agent runs |
|---|---|
| "start the dev server", "serve the app", "start the backend", "run the API", "spin up the worker" | `tracks up [service]`; with several services and no clear match, it asks which |
| "stop it", "shut the backend down" | `tracks down [service]` |
| "is it running?", "what's the URL?", "which port?" | `tracks services`, `tracks url <service> [port]` |
| "show me the logs", "why did it crash?" | `tracks logs <service>` |
| "proxy it", "make it the one I test", "switch localhost to this track", "start the backend and proxy it" | `tracks proxy use [port or group]`, or `tracks up <service> --proxy` |

The agent never starts a service itself (`pnpm dev`, `sbt run`): Tracks
wouldn't own it, and couldn't stop or proxy it.

### Switching the proxy from a track

- `tracks proxy use [port or group]`, run in a track, points that output port
  or group at this track: a simple port at this track's service, a group's
  input at this track. `tracks up <service> --proxy` starts and switches in one
  command.
- **Deferred, not blocking.** The command returns at once; the daemon switches
  when the service is ready, so a long sbt compile never overruns the agent's
  command timeout. `--wait` waits up to a short timeout for agents that want to
  report the result. A pending switch shows on the Proxy tab and in `tracks
  services`, and is dropped if the service stops first.
- **Which port.** Without a name, the one port or group that can take this
  track's service is chosen. With several, the command lists them and the
  agent asks the user.
- **Taking it from another track** is allowed: the user asked. The other
  track's service keeps running. The Proxy tab shows who switched last
  ("switched by track-x · 2 min ago"), and the user gets a notification.
- **Pinned.** A port or group the user pins on the Proxy tab can't be switched
  from a track; the command says it's pinned, and by whom it's held.
- The prompt says to switch only when the user asks, never on the agent's own
  initiative.

Two tracks on the same Scala backend both run `api`. The user tells track B
"proxy it"; B's agent runs `tracks proxy use payments`, and once B's `api` is
ready the `payments` group moves HTTP and gRPC from A to B in one switch, while
A keeps running.

## Logs

`tracks logs` reads the pane's scrollback, which a busy backend fills within
minutes, and agents debugging a service need its history. A service with **Log
file** on also writes its output to `<state>/logs/<track>/<repo>-<service>.log`,
through tmux's `pipe-pane`, so the process keeps its terminal and its colours.

- Each file keeps its last 10 MB; it's removed when the track ends.
- `tracks logs <service>` reads the file when there is one, the scrollback
  otherwise. `--grep <pattern>` filters, `--since 5m` cuts by time, from the
  time Tracks stamps on each line it writes.
- In the file: `log: true` on a service.

## Built-in docs

Short topics that explain Tracks where it's used, one source for the user and
for agents.

- **Source.** Markdown topics embedded in the binary (`internal/docs`), so they
  always match the installed Tracks: `tracks` (an overview), `config`,
  `services`, `proxy`, `checks` and `recipes`. Each is brief: what it's for, its
  keys or fields, one example.
- **CLI.** `tracks docs` lists the topics; `tracks docs <topic>` prints one.
  `tracks docs config --schema` prints the file's JSON Schema, for editors.
  `tracks config check [file]` validates a config file and reports each error
  with its line.
- **UI.** Quick Access (`Ctrl+b q`) gets **Docs**: a popup with the topics on
  the left and the text on the right, drawn from theme tokens with no new
  dependency. In the repo form, `?` opens the topic of the focused section at
  that section.
- **Agents.** Work and Review prompts get one line: Tracks' docs are in
  `tracks docs`, to be read before changing `.tracks/config.yaml`, and `tracks
  config check` must pass after. Golden change. Claude gets a `tracks-config`
  skill, installed with the others, that triggers when asked to set up a repo
  for Tracks, its services or its proxy presets; Cursor gets the same as a rule.
- **No drift.** A test parses every YAML example in the topics with the real
  loader, and checks that every key the loader knows has a topic that names it.
- Every step below that adds a key adds it to its topic in the same PR.

## Done when

The roadmap is done when these hold, on a clean machine, end to end:

1. A Scala service with HTTP, gRPC and a Compose Postgres, set up only by its
   `.tracks/config.yaml`, runs in two tracks side by side. Each track has its
   own containers and migrations, and one group switch on the Proxy tab moves
   HTTP and gRPC from one track to the other.
2. A Python service (uv, FastAPI) and a Kafka consumer with no port do the same,
   the consumer showing as ready while it runs.
3. A frontend and its backend in one track: the frontend waits for the backend
   across repos and reads its port from `$TRACKS_PORT_…`.
4. In each, the agent finds Tracks' docs with `tracks docs`, writes or changes
   the config file, passes `tracks config check`, and runs the repo's checks
   before saying it's done.
5. A frontend repo that never had a config file behaves exactly as before the
   roadmap, and an existing Tracks database upgrades with nothing lost.

## Testing Tracks

- End-to-end tests use processes that aren't Node: `python3 -m http.server` for
  HTTP, and small Go helpers in the tests for TCP, gRPC and a service with no
  port. CI needs no JVM and no Docker.
- Docker paths (Compose readiness, teardown) get a test that skips without
  Docker, plus the field test.
- Each step's PR adds its end-to-end test with these helpers, so the paths
  that aren't frontend stay tested once they ship.

## Steps

### 0. Field test (no code)

- Configure one Scala (sbt) service and one Python (uv or poetry) service as
  repos: a setup, a service on an assigned port.
- Run two tracks of the same repo side by side; add an output port; let the
  agent build, test and run the service.
- Check in a setup and a service pane which JDK and Python run, against the
  repo's `.envrc`, `mise.toml` or `.java-version`.
- Time a cold track to its first ready service, and note memory with two and
  three tracks running.
- Run `docker compose up` as a service and check whether Tracks ever sees it
  ready (see step 7).
- Time the daemon's `lsof` and `ps` poll with several JVMs running, since step
  7 adds checks to every poll.
- Note every gap under Findings; adjust the steps.

### 1. Quick fixes

| Change | Where |
|---|---|
| `COMPOSE_PROJECT_NAME=<repo>-<track id>` for setup, services and the agent | `tracks/servers.go`, setup, agent pane env |
| A listener on the command line's `jdwp=…address=<port>` is never the service's port | `tracks/detect.go` `listening` |
| Service types `sbt`, `uvicorn`, `gunicorn`, `hypercorn`, `django`, `flask`, `nest`, `tsx` | `procs.Kind` |
| Examples from several stacks in placeholders and hints | repo form, `cli/setup.go` |
| Docs-only list names docs files exactly (no `*.txt`), adds `*.rst`; neutral Services example | agent prompts. Golden change: agree first |

### 2. Rename to services

Code and database say service, with nothing lost on upgrade.

- **Code.** `DevServer`, `RoleDevServer`, `tracks/servers.go` and the rest
  become `Service`, `RoleService`, `tracks/services.go`; about 20 files,
  mechanical.
- **Database.** One new migration builds `services` with `port_mode`'s CHECK
  widened to `none`, copies every row of `dev_servers`, and drops it.
  `proxy_ports.input_server` becomes `input_service`. Applied migrations stay
  as they are.
- **Backup.** Before applying any migration, Tracks copies `tracks.db` to
  `tracks.db.bak-<version>`, so an older binary can be brought back by hand.
  It keeps the last three.
- **Port mode none:** ready while running, no `$PORT`, never in the proxy.
  Detection and the repo form learn it here, with the migration it needs.
- **UI wording:** Services in the repo form and the Proxy tab; `tracks services`.

Compatibility, each kept for one release and then removed:

| Part | Old name still read |
|---|---|
| Panes started before the update | tmux role `dev-server`, so their processes are still found and stopped |
| RPC, while `tracks update` mixes an old daemon and a new CLI | method `servers` next to `services` |
| CLI | `tracks servers` stays for good, as another name for `tracks services` |

Golden change: the prompts' Dev servers paragraph becomes Services, with
step 1's change.

### 3. Repo config file, existing settings

- Load, validate and trust `.tracks/config.yaml` with today's keys: `setup` and
  `services` (name, command, dir, port mode and number, type).
- `tracks docs`, the `tracks` and `config` topics, `tracks config check`, the
  prompt line and the `tracks-config` skill. Golden change.
- The source switch (Repo file / Manual), **Detach to manual**, **Export to
  file**, **Switch to repo file** and the upstream-change notice.
- Personal fields: fixed ports, default services and extra environment
  variables.
- The Koda fallback for `setup`.
- Migration: `repos.source`, the personal fields, and the accepted file hashes.

### 4. Worktree parity

- `copy` and **Copy into worktrees** (`workspace.CopyEnv` becomes
  `CopyIgnored(patterns)`), `submodules`, `stop_grace`.
- `warm`: copy-on-write clones of ignored folders, before setup, so setup
  finds a warm build or installed dependencies.
- `toolchain`: setup, services, checks and teardown run through `direnv
  exec` or `mise exec`. The agent pane gets the same environment.

### 5. Named ports

- Extra ports and protocol on services; the 20-port block; `tracks services` and
  `tracks url api grpc` list them; cross-repo port variables.
- Prompts: the Services paragraph mentions extra ports. Golden change.

### 6. Proxy groups and modes

- Names on simple ports; groups with a track as input; HTTP and TCP modes;
  **Add from repo**. `proxy_ports` becomes groups and their ports, in a
  migration that turns each existing port into a group of one.
- Switching from a track: `tracks proxy use`, `tracks up --proxy`, the
  deferred switch, "switched by", pinning and the notification. RPC
  `proxy.use`. The prompt's Services paragraph with the requests it maps, and
  the `services` and `proxy` docs topics. Golden change.

### 7. Backing services, readiness and logs

- `after` on services and on setup: a service named in `setup.after` starts
  without waiting for setup, and setup waits until it's ready.
- `after` across repos: `payments-api/api` names a service in another repo of
  the same track.
- **Ready without the process tree.** A service with an assigned or fixed port
  is ready once a TCP connect to that port succeeds, whoever holds it: the
  port was reserved for this service. `docker compose up` needs this, since
  the port is held by Docker's own process, not by one below the pane. Detect
  mode still reads the process tree.
- `ready`: an HTTP path or an output pattern, checked after the connect
  succeeds. Wired end to end in this step: state, `tracks services`, `after` and the
  proxy's state all use it.
- **Log file**: `pipe-pane`, the size cap, and `tracks logs` reading it with
  `--grep` and `--since`.
- `teardown`, and a Compose recipe that works end to end with step 5's ports
  and a migration in setup.

### 8. Checks and review

- `checks` and `docs` in the file, with the Koda fallback for `lint` and
  `test`. The agent prompt names the repo's checks; the review rule uses
  `docs`. Golden change.
- Reviewer convention discovery also reads `CLAUDE.md`, `README*`, `build.sbt`,
  `.scalafmt.conf`, `.scalafix.conf`, `pyproject.toml`, `setup.cfg`,
  `tox.ini`, `tsconfig.json`, `eslint.config.*`. Golden change.

### 9. Docs in the UI and recipes

- **Docs** in Quick Access, and `?` in the repo form.
- The `recipes` topic: config files for an sbt service (Play and http4s/Akka
  HTTP), a Python service (FastAPI with uv, Django with poetry), a Nest
  service, a frontend with Storybook, and a Compose stack.

## Order

| Step | Size | Schema | Golden |
|---|---|---|---|
| 0 Field test | S | – | – |
| 1 Quick fixes | S | – | yes |
| 2 Rename to services | M | yes | yes |
| 3 Repo config file | L | yes | yes |
| 4 Worktree parity | M | yes | – |
| 5 Named ports | L | yes | yes |
| 6 Proxy groups | L | yes | yes |
| 7 Backing services, readiness and logs | L | yes | maybe |
| 8 Checks and review | M | – | yes |
| 9 Docs in the UI and recipes | M | – | – |

Steps 4–8 each add their keys to the file, their fields to the form and their
part of the docs topics together.

## Later, optional

Not planned in the steps above; written down so they fit the design when
wanted.

- **Restart on crash.** `restart: on-failure` on a service, off by default:
  restarted after it exits non-zero, with a growing wait (1 s, 2 s, 4 s … up to
  30 s), and given up after five failures in a row, which is shown as today's
  crash. Many stacks already restart inside the process (sbt-revolver's
  `~reStart`, `uvicorn --reload`, `nodemon`), so it's opt-in.
- **Path routing** in HTTP mode, as under Proxy.

## Open

- `COMPOSE_PROJECT_NAME`: always, or only when a repo opts in, so a compose
  file's own `name:` keeps winning?
- `.tracks/config.yaml` or a single `.tracks.yaml`?
- Should a repo's accepted file be trusted for every branch, or per content
  hash as above?
- One shared Postgres for all tracks, or one per track? Both work; which does
  the recipe show first?
- Which Scala and Python repos for the field test?
- Machine load: limit how many setups run at once, and show each track's
  memory in the footer? Decided after the field test's numbers.

## Findings

Filled in by step 0.
