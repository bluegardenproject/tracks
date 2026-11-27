# Agent protocol

**Status:** v2, planning. Plugins' shape, Tracks' MCP server and how
plugins reach the relay were decided on 2026-11-27 ("Decided"), and a
spike showed how Cursor gets Tracks' MCP server. The envelope and
programmable messages are still ideas. Built on the relay and the trains of
[trains.md](trains.md), and on its Phase 2, side tracks.

## Problem

- **The relay is a protocol nobody wrote down.** Handles, kinds, the star,
  fenced output, the pointer, `send|read|wait|who`: agents learn it from a
  paragraph in their prompt, and a new feature is another paragraph.
- **Agents are built in.** The reviewer is Tracks' own. Developers will
  want their own: a migration checker, a release note writer, a
  dependency watcher, a planner that comes with its team's context.
- **Messages only carry words.** "Rebase once #232 merges", "nobody edit
  relay.go while I do", "merge in this order once CI is green": today an
  agent writes them, another has to read, remember and act, and nothing
  holds anyone to them.
- **The user has to stay in the loop** however much the agents do among
  themselves.

## Not in scope, yet

- Agents across machines. The relay is local; so is this.
- **The conductor**: global watchers above the tracks, on a relay of
  their own (an `@station`), e.g. a dependency watcher over every
  track. An idea for later; v2's plugins live inside one track and its
  side tracks.
- Plugin kinds other than `train` and `track` (a theme, an engine, a
  policy).
- **A sandbox for plugins' code** (its MCP servers, context providers,
  event handlers): Tracks starts it as the user, outside the engines'
  permission prompts and sandboxes. Dropped for v2 (2026-11-27), to be
  discussed as a follow-up, with the sandbox for a command-running train
  (trains.md, "Open") and tools such as
  [Sandcat](https://sandcat.virtuslab.com/latest/index.html): a profile
  from the manifest (read the worktree, write the plugin's own folder,
  reach its declared `hosts`) on macOS and Linux.

## Decided (2026-11-27)

| | Decision |
|---|---|
| Where plugins live | First-party ones in a separate `tracks-plugins` repo, a folder each; later any git repo, holding one plugin or many |
| What a plugin is | A package of **components**, each with a `kind`: `train` (an agent a track starts) or `track` (a track type beside the built-in ones). One plugin may bring a track type and the trains it uses |
| Track types | Extend a built-in **base** only (`base: plan`): they inherit its worktrees, read-only mode and permissions, and add a brief, context, actions and the trains they suggest. No permissions of their own, for now |
| Code | Allowed, in slots the manifest declares, nowhere else (see "Code in plugins") |
| Trust | Nothing runs until the user approves: install pins a commit and shows the permissions, the hosts and the code's entry points; an update shows the diff and asks again when the code or the permissions changed. No sandbox for plugins' code in v2: like npm packages or editor extensions, it runs as the user once approved (see "Not in scope, yet") |
| The Reviewer | Stays built in; moves over last, as a plugin bundled in the binary, the strictest test of the interface |
| Managing them | A **Plugins** tab in the Tracks window, before Settings: add, update, remove, disable, and each plugin's settings (drawn from its manifest). The CLI does the same: `tracks plugin add/list/update/remove` |
| The relay | Plugins get the rights built-in agents have, no more: the star, the side tracks' rules (see "Plugins on the relay") |
| Who answers for what | Tracks keeps its **own** trains safe: the Reviewer and the built-in trains stay read-only and cut off from the user's MCP servers (`--strict-mcp-config`), so they can't post to Slack, comment on a PR or push. A **plugin** does what its author built and its user approved: a reviewer plugin that posts on GitHub or Slack is a valid plugin. Tracks shows a plugin's declared permissions on install and enforces its own tools' rules (the relay), but doesn't police what a plugin does with the user's MCP servers |
| Tracks' MCP server | Built **first**: every engine needs it, and it's how plugins' own MCP servers reach agents, as a gateway (see "Tracks as an MCP server") |
| Engines | Claude and Cursor both (Cursor through its plugin folder, see the spike); Copilot, Codex and others through the same server once they're engines |

## Plugins

A plugin is a folder with a `plugin.yaml`:

```yaml
protocol: 1                     # Tracks refuses another major, warns on a newer minor
name: acme-planning
engines: [claude, cursor]       # for engine assets; omitted means both
settings:                       # drawn in the Plugins tab
  - { name: adr_dir, type: string, default: docs/adr }
components:
  - kind: track
    name: custom-planner
    base: plan
    brief: briefs/planner.md
    context: [ctx/adrs]         # context providers, below
    trains: [research]          # trains it suggests to its agent
    actions: [...]              # footer buttons
    defaults: { engine: claude, model: opus }
  - kind: train
    name: research
    lifecycle: one-shot         # or long-running
    permissions: read           # read | commands | write (write only as a side track)
    hosts: ["*.atlassian.net"]
    brief: briefs/research.md
    mcp: [servers/jira]         # its own MCP servers, through the gateway
    requires_mcp: [datadog]     # servers the user has set up, with their credentials
    relay:
      sends: [report, event, question]
      reads: true
      subscribes: [github.ci_failed, train.done, relay.to_me]
```

### Code in plugins

Code runs only in these slots, in any language, with a timeout per call,
its contract versioned by `protocol`:

| Slot | What the code does | Contract |
|---|---|---|
| MCP servers | Gives its agents tools of their own | Standard MCP, started by Tracks' gateway, never by the engine |
| Context providers | Runs when a track or train starts; its output goes into the prompt, fenced as data | JSON in (track, repos, task), text out |
| Event handlers | Runs on the events the component subscribes to, and decides | An envelope in; `deliver`, `notify`, `ask_user` or nothing out, within what the manifest declared. It never writes to the relay itself |
| Engine assets | Skills, subagents, commands, rules for its agents | Files Tracks installs per session, as it does the reviewer's subagents and Cursor's hooks plugin |

A long-running train such as the Watcher is a lifecycle Tracks runs: its
handlers wake on events or a timer, and an agent only when there's
something to think about.

`requires_mcp` names servers the plugin needs but doesn't ship: a
Datadog plugin uses the user's own Datadog server and credentials.
Tracks already lists each engine's servers (`agents.ListMCP`); the
Plugins tab says which are missing, for which engine.

### Install, update, trust

- `tracks plugin add <git url>[@ref]`, or a local path while developing
  one; pinned to a commit.
- The approval screen: the components, their permissions and hosts, the
  code's entry points, the MCP servers it ships and requires.
- `tracks plugin update` shows the manifest's and the code's diff; a
  change to either asks again.
- Disabled plugins stay installed and are hidden from tracks.

### Plugins on the relay

The relay stays a star per track; plugins add no rules:

| Part | On the relay as | Rules |
|---|---|---|
| A `train` component | An instance with its own handle (`research-123`), as the reviewer | Sends to its track, reads what's addressed to it; its identity comes from its MCP token |
| A `track` component | A track like any other, with its own relay | As work and plan tracks, side tracks included |
| Code | Nothing of its own: a handler returns an envelope, which Tracks checks against the manifest and posts under the instance's handle | The kinds and recipients the manifest's `relay` declares |

Messages to and from plugins are data: fenced for agents, plain JSON for
code, never instructions. The Relay Station shows a plugin's messages as
any other's, with the plugin's name beside the handle.

### The first plugins

In order, each adding one capability:

1. **Investigator**: one-shot, read-only; the reviewer's shape.
2. **Research agent**: one-shot, read-only, several at once, one per
   topic; needs `hosts` (a train's sandbox, trains.md "Open").
3. **Watcher**: long-running; needs event handlers.

## Many instances, picked by the track

Every train, built in or a plugin, runs as several instances in one
track, or side track, at once: three reviewers on three checkouts, a
research agent on four topics. The track's agent picks the agent a task
needs and starts as many as it takes. So a train's state (its folder, its
hidden session, its run marker, its place on the relay) is per instance,
keyed by its handle, for every kind alike; the manifest says what an
agent is, not how many may run. Several reviewers in a track (one per
target, [review.md](review.md)) is the first.

The track's agent finds them with `agents_list` (an MCP tool, and the
CLI), which returns the built-in trains and the installed plugins'
manifests; it starts each with `tracks train new <kind> --task …`. No
cap; a setting may ask the user before more than N run.

## Three layers, and a fourth to invent

### 1. The manifest: what an agent declares

Decided: see "Plugins" for the manifest and what Tracks enforces.

### 2. The envelope: a message, versioned

A message keeps its free-text body, always fenced as data, and gains the
fields Tracks acts on:

```json
{ "v": 1, "id": 42, "from": "watch-000123", "to": "track", "kind": "event",
  "at": "…", "outcome": "blocked",
  "refs": { "prs": ["https://…/pull/234"], "files": ["internal/relay/relay.go"] },
  "body": "CI failed on lint: …" }
```

- `outcome` sets the badge: no more parsing `REVIEW OUTCOME:` lines.
- `refs` link PRs in the Relay Station and feed overlap detection.
- Today's messages are the envelope without the new fields: agents of
  today keep working.
- Built: `outcome` (`pass`, `blocked`, `done`, `needs-input`) and `refs`
  (PR URLs, `#123` for a PR of the track's repo, repo-relative files; at
  most 20), checked in `internal/relay`; `tracks relay send --outcome
  --ref`; the reviewer's report and `@github`'s events set them.

### 3. Tracks as an MCP server: how agents use Tracks

Decided, and built first. `tracks mcp` is a stdio server each agent's
engine starts; it talks to the daemon over its socket. Tracks offers its
features as typed tools instead of commands described in prompts:
`relay_send`, `relay_read`, `relay_wait`, `relay_who`, `proxy_status`,
`panes_list`, `review_ask`, `agents_list`, `train_new`, …

- **Identity**: Tracks gives each agent instance a token of its own, in
  its pane's environment; the server reads it, so the sender is certain
  (the tokens trains.md leaves for later, solved for all) and the
  engine's config holds no secret.
- **Capabilities**: the server shows an agent only the tools its
  manifest lists, by its identity.
- **The gateway**: a plugin's own MCP servers are started by Tracks'
  server, not the engine, and their tools passed through under the
  server's name (`jira_search`), only to the agents of that plugin's
  components. The engine knows one server, `tracks`, whatever plugins
  are installed: no per-plugin config, nothing written into a worktree,
  the same for every engine, and every call checked against the
  manifest and audited. Remote servers that log in with OAuth would
  need Tracks to run the login: open.
- **Trust, automatic**: the engine trusts one server, set up by Tracks;
  the plugins behind it are trusted by the user's approval on install.
  - Claude: `--mcp-config` per session (servers from it get no project
    approval prompt), and `mcp__tracks` allowed in the `--settings` file
    Tracks already passes.
  - Cursor: no per-session MCP flag, but the hooks plugin Tracks
    already passes with `--plugin-dir` can bring MCP servers: a
    `.mcp.json` at its root. It's trusted without a prompt; never pass
    `--approve-mcps`, which approves the repo's own servers too and
    remembers it. Tool calls run without asking under `--force` (work
    and review tracks); plan and ask tracks ask once, and trains get an
    allow rule, `Mcp(<server id>:*)` (see "The spike: Cursor").
- **Discovery**: tools carry their schemas; the prompts shrink.
- **Receiving** stays the pointer, then `relay_read`: the agent CLIs don't
  act on a server's notifications.
- The CLI stays for engines without MCP.

#### The spike: Cursor

Done on 2026-11-27, with `cursor-agent` 2026.11.22. It works through
the plugin folder; nothing goes into `~/.cursor/mcp.json`.

| Question | Finding |
|---|---|
| Does a `--plugin-dir` plugin load MCP servers? | Yes: `<plugin>/.mcp.json` (or `mcp.json`), `{"mcpServers": {…}}`, at its root. The server's id is `plugin-<plugin folder>-<server>`. Skills (`skills/<name>/SKILL.md`) load too; rules (`rules/*.mdc`) are read but weren't seen in context |
| Trusted without a prompt? | Yes, with no flag. A repo's `.cursor/mcp.json` server in the same run doesn't start; `--approve-mcps` starts it and records the approval in `~/.cursor/projects/<workspace>/mcp-approvals.json` |
| Does the pane's environment reach the server? | Only HOME, PATH, SHELL, TERM, TMPDIR, USER and a few more, plus the config's `env`: `"env": {"TRACKS_AGENT_TOKEN": "${env:TRACKS_AGENT_TOKEN}"}` passes it. Each session starts its own server process, so parallel sessions each get their own token |
| Do tool calls ask? | Not under `--force`. Under `--mode plan`, `--mode ask` and `-p`, they're rejected (`-p`) or ask "Run this MCP tool?" (interactive), unless `<workspace>/.cursor/cli.json` allows them: `{"permissions": {"allow": ["Mcp(<server id>:*)"]}}`, one server's tools only |
| Does `--add-dir` give read access? | Reads aren't confined with Cursor's sandbox off: a plan-mode agent read outside its workspace with and without it. It matters only with the sandbox on (not tried) |
| Other | `cursor-agent mcp list` ignores `--plugin-dir`. MCP tools load lazily |

The setup, one file added to the hooks plugin (the folder named
`tracks`, so the server's id is `plugin-tracks-tracks`):

```
<hooks>/tracks/.cursor-plugin/plugin.json   {"name": "tracks-hooks"}
<hooks>/tracks/hooks/hooks.json             as today
<hooks>/tracks/.mcp.json                    {"mcpServers": {"tracks": {
    "command": "/abs/path/tracks", "args": ["mcp"],
    "env": {"TRACKS_ID": "${env:TRACKS_ID}",
            "TRACKS_AGENT_TOKEN": "${env:TRACKS_AGENT_TOKEN}"}}}}
```

Plan and ask tracks run in the user's checkout without `--force`, so
Cursor asks once, in the track's pane, before the first `tracks` tool
call ("Run once / Allowlist MCP Tool / Reject / Skip"). Settled
(2026-11-27): that's fine, since those tracks are interactive. Trains
run headless, where an unanswered call is rejected, but in Tracks' own
folders, where Tracks writes `.cursor/cli.json`'s allow rule itself.
Later, to save the question: a hook that allows only `tracks` calls
(Cursor's `beforeMCPExecution`, Claude's `PreToolUse`), which Tracks
already installs hooks for.

The same holds for a server a plugin requires (`requires_mcp`, e.g. the
user's Datadog): Cursor and Claude ask as they would without Tracks;
the user set the server up and has the say.

### 4. Programmable messages: to invent

The first three layers are plumbing. This is where the protocol can do
something new: **a message that carries logic**, which Tracks runs, not
the agents, with the user as the final say.

#### The idea

A message carries, beside its words, a small contract: **when** something
happens (a trigger), **if** something holds (a guard), **do** something
(an effect). Tracks evaluates it; delivering or reading the message, or an
event later, runs it.

```yaml
kind: proposal
body: "I'll rework relay.go; hold your edits to it until my PR merges."
contract:
  lease: { files: ["internal/relay/relay.go"], holder: side-000123, until: "pr(side-000123).merged" }
  on_conflict: { notify: [holder, track] }
```

```yaml
kind: task
to: side-000456
body: "Rebase onto main and re-run the tests."
contract:
  when: "pr(232).merged"
  do: deliver            # the message waits, then delivers itself
```

```yaml
kind: proposal
body: "Merge order: #232, then #233, then #235."
contract:
  when: "all(ci_green(232, 233, 235))"
  do: { ask_user: "Merge in this order?", then: merge_in_order }
```

#### Kinds of contract worth trying

- **Deferred delivery**: a message that waits for an event (a PR merging,
  CI going green, a train finishing, a time) and delivers itself then.
- **Leases**: a claim on files or a branch, with an expiry, that Tracks
  checks against the agents' changes and the overlap notices; side
  tracks' conflicts turn into agreements made up front.
- **Commitments**: "I'll report by 16:00, or by the time CI finishes";
  missing one is a signal, to the main track or the user.
- **Read receipts with consequences**: on read, ask for an answer within a
  time, or escalate to the user.
- **Proposals and votes**: side tracks agree on an order, a split, a
  design; the user breaks ties or approves.
- **Gates**: an effect that needs the user's approval, shown in the Relay
  Station as Approve / Deny, and waits for it.

#### Rules that keep it safe

- **Tracks runs contracts, never an agent's code.** A message from one
  agent can't run anything in another's session: that's the prompt
  injection the fenced-data rule keeps out today.
- **A small vocabulary, not a language**: a fixed set of triggers (merged,
  CI, train done, read, delivered, time), guards over Tracks' own state,
  and effects (deliver, notify, lease, ask the user, set a badge, merge
  after approval). New ones are added to Tracks, not written in messages.
- **Effects within the sender's manifest**: a contract can't do what its
  sender couldn't.
- **The user's gate**: whatever changes code, merges, or reaches outside
  the machine asks the user first, in the Relay Station or as a
  notification. The user can cancel any contract, any time.
- **Audited**: every trigger and effect is a notice on the relay, so the
  Relay Station shows what ran, when and why.

#### Other directions to keep open

- **A shared board** instead of, or beside, messages: tasks, claims and
  their states the agents read and update, as a team's board, with the
  relay for the talk around it.
- **Typed channels**: a channel per concern (a PR, a file, a release)
  rather than per agent.
- **Negotiation as a first-class thing**: proposals, counter-proposals and
  acceptance, with the user as the one who can always decide.

## Plugins, settings and policies

The Reviewer is to become a plugin, and Tracks' first policies (the
GitHub ones of [trains.md](trains.md), "Side tracks and GitHub's events")
decide what happens on an event. A plugin needs both, through the
interface, in both directions:

- **Plugin → Tracks, in its manifest**: its settings as a schema (name,
  type, values, default: `ask_before_human_replies: bool = true`), which
  Tracks draws in the Plugins tab under the plugin's name; the events it
  subscribes to (`github.conflicts`, `github.ci_failed`,
  `github.review_threads`, `review.requested`, …); and the effects it
  may answer with (instruct a track, ask the user, post a report), so it
  can't do more than it declared.
- **Tracks → Plugin**: events as envelopes (refs to the PR and the
  track, facts, never a comment's text), and the plugin's own settings'
  values, so it reads no config of Tracks'.
- **A policy is a contract**: "conflicts → handle automatically" is
  `when: github.conflicts(pr) do: deliver(side, "Rebase …")`, and "ask
  me" swaps the effect for a gate. Built as a fixed table of triggers and
  effects that Tracks evaluates, Settings → GitHub can become a built-in
  plugin drawn as any other, a change of who declares the rules, not a
  rewrite.

Open: whether Settings → GitHub moves into the Reviewer's plugin or
stays Tracks' own; how a plugin's settings are validated and migrated
across its versions.

## Order

1. The spike: Cursor and MCP. Done.
2. Tracks' MCP server for what exists: the relay, the proxy, the panes;
   identity per instance; `agents_list`.
3. In parallel, apart from 2: the envelope's fields (`outcome`,
   `refs`); the manifest and the plugin store (`tracks plugin …`,
   approval, the Plugins tab).
4. On 3: a registry of train kinds and one of track types, in place of
   the hard-coded `train.Reviewer` and `track.Kinds`.
5. On 2 and 4: the gateway for plugins' MCP servers, then context
   providers, then event handlers.
6. The first plugins, in `tracks-plugins`: the Investigator, the
   research agent, the Watcher.
7. Programmable messages: deferred delivery and leases first, the two
   that pay off with side tracks, then the user's gates.

## Open

- Whether a plugin train gets all the user's MCP servers, as an ordinary
  session does, or only those its manifest names: the user's servers
  are the plugin's and the user's business, not Tracks'.
- OAuth for remote MCP servers behind the gateway.
- Follow-up discussion: sandboxing plugins' code and command-running
  trains, Sandcat among the options ("Not in scope, yet").
- How a plugin's settings are validated and migrated across its
  versions.
- Contracts: YAML in a message, or a tool call (`relay_send` with a
  `contract` argument) that Tracks validates against the vocabulary?
- How a contract shows in the Relay Station: waiting, running, done,
  cancelled; and how the user cancels or approves one.
- Leases: enforced (Tracks refuses an agent's change to a leased file, as
  the worktree check catches edits) or advisory (a notice when broken)?
- How much of A2A (Google's open Agent2Agent protocol: tasks, their
  states, message parts) to borrow, while keeping the relay local and
  simple.
