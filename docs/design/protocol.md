# Agent protocol

**Status:** v2, exploring. Nothing is decided; this collects the ideas and
the questions to plan and discuss. Built on the relay and the trains of
[trains.md](trains.md), and on its Phase 2, side tracks.

## Problem

- **The relay is a protocol nobody wrote down.** Handles, kinds, the star,
  fenced output, the pointer, `send|read|wait|who`: agents learn it from a
  paragraph in their prompt, and a new feature is another paragraph.
- **Agents are built in.** The reviewer is Tracks' own. Developers will
  want their own: a migration checker, a release note writer, a
  dependency watcher.
- **Messages only carry words.** "Rebase once #232 merges", "nobody edit
  relay.go while I do", "merge in this order once CI is green": today an
  agent writes them, another has to read, remember and act, and nothing
  holds anyone to them.
- **The user has to stay in the loop** however much the agents do among
  themselves.

## Not in scope, yet

- Agents across machines. The relay is local; so is this.

## Planning: the agenda

Planning started on 2026-11-25, with side tracks (trains.md, Phase 2)
released in 3.9.0. The topics, in this document's sections, and the
order to discuss them in (not the order to build them, which is under
"Order, once it's decided"):

| | Topic | Where |
|---|---|---|
| A | What a plugin is: its shape (A1, manifest-only or executables), where it lives, install and update, compatibility, trust, the Reviewer as a plugin | Plugins first |
| B | The manifest: what an agent declares | The manifest |
| C | The envelope: a message, versioned | The envelope |
| D | Tracks as an MCP server: how agents use Tracks | Tracks as an MCP server |
| E | Programmable messages | Programmable messages |
| F | Many instances, and how the track finds and picks agents | Many instances |
| G | Plugins' settings and policies, both ways | Plugins, settings and policies |
| H | The first plugins: the Investigator, the Watcher, a research agent | Plugins first |
| I | A command-running train's sandbox: switched on without managed settings, the hosts it may reach | trains.md, Open |

Discussion order: **A1 first**, since manifest-only or executables
decides the trust model, the install story and what the Watcher can be;
then **D**, how plugins act; **B** and **C** follow from those two; **E**,
the open design space, once the ground is set; F to I along the way.

**Where it stands**: at A1, with this proposal on the table, not decided:

- Plugins start **manifest-only**: a brief and the Tracks tools it
  declares, no code of its own. Everything a plugin does goes through
  Tracks, which enforces and audits its permissions.
- A long-running agent such as the Watcher is a **lifecycle Tracks runs**,
  not a loop the plugin ships: Tracks wakes it on the events it
  subscribes to (an `@github` event, a train finishing, a relay message)
  or on a timer it declares, and it answers through its tools.
- **Executables** may come later, behind an explicit trust step of their
  own, if a plugin turns out to need code (a parser, a protocol client).

## Plugins first: the Investigator and the Watcher

The Investigator and the Watcher (trains.md, formerly its step 12) move
here: they're built as the first plugins, on the open interface below,
rather than as two more built-in agents to migrate later. They may live
outside the `tracks` repo, in a repo of plugins.

To plan and discuss before building:

| Area | Question |
|---|---|
| What a plugin is | A manifest and a brief only, so no code of its own runs; or may it ship executables (a watcher loop, a parser)? Manifest-only is far safer and may cover most cases. |
| Where it lives | A repo of first-party plugins (e.g. `tracks-plugins`), and any git repo for others; one repo per plugin, or one for many? |
| Install and update | `tracks plugin add <git url>`, pinned to a tag or commit, an update command, listed in Settings → Trains (agents) |
| Compatibility | A plugin declares the protocol version it speaks; Tracks refuses or warns on a mismatch |
| Trust | What a stranger's plugin may do by default (read only?); the user approves its permissions on install, as a phone's app's |
| The Reviewer | Stays built in, or becomes a plugin once the interface exists, as its strictest test |
| Settings | Each plugin's engine, model and prefix, as the Reviewer's frame has |

A first lean, not decided: manifest-only plugins at first, a repo of
first-party ones, installed from git and pinned, permissions approved on
install, the Reviewer moved over last. The fuller A1 proposal is under
"Planning: the agenda".

A third candidate for the first plugins: a **research agent**, run as
several instances at once, one per topic (see "Many instances"). It
reads and reports, as the Investigator does, and may share its manifest
shape.

## Many instances, picked by the track

Every train, built in or a plugin, runs as several instances in one
track, or side track, at once: three reviewers on three checkouts, a
research agent on four topics. The track's agent picks the agent a task
needs and starts as many as it takes. So a train's state (its folder, its
hidden session, its run marker, its place on the relay) is per instance,
keyed by its handle, for every kind alike; the manifest says what an
agent is, not how many may run. Several reviewers in a track (one per
target, [review.md](review.md)) is the first.

Open: how the track's agent **finds** the agents it can start (the
built-in trains and the installed plugins, each with what it's for and
what it costs), and how it starts several at once (`tracks train new
<kind> --task …` per instance, or one call for a set), with the user's
say in how many run. Tracks as an MCP server would answer the first
with a tool that lists them, with their manifests.

## Three layers, and a fourth to invent

### 1. The manifest: what an agent declares

A plugin is a file (or a Settings entry) that declares what the agent is.
Tracks enforces it: an agent never reaches for more than it declared.

| Field | Example |
|---|---|
| Name, prefix | Watcher, `watch` |
| Engine and model, as defaults | Claude, sonnet |
| Brief | its instructions; the task goes after them |
| Lifecycle | one-shot (reports, then done), long-running (events until a condition) |
| Permissions | reads only / runs commands / writes (a side track) |
| Tools | `relay`, `proxy.read`, `panes.list`, `github.read`, … |
| Contract | the kinds it sends, its outcomes (`done`, `blocked`, `needs input`) |

### 2. The envelope: a message, versioned

A message keeps its free-text body, always fenced as data, and gains the
fields Tracks acts on:

```json
{ "v": 1, "id": 42, "from": "watch-000123", "to": "track", "kind": "event",
  "at": "…", "outcome": "blocked",
  "refs": { "pr": "https://…/pull/234", "files": ["internal/relay/relay.go"] },
  "body": "CI failed on lint: …" }
```

- `outcome` sets the badge: no more parsing `REVIEW OUTCOME:` lines.
- `refs` link PRs in the Relay Station and feed overlap detection.
- Today's messages are the envelope without the new fields: agents of
  today keep working.

### 3. Tracks as an MCP server: how agents use Tracks

Both engines speak MCP. Tracks offers its features as typed tools instead
of commands described in prompts: `relay_send`, `relay_read`,
`relay_wait`, `relay_who`, `proxy_status`, `panes_list`, `review_ask`,
`train_new`, …

- **Identity**: each agent's server starts with a token of its own; the
  sender is certain (the tokens trains.md leaves for later, solved for
  all).
- **Capabilities**: an agent sees only the tools its manifest lists.
- **Discovery**: tools carry their schemas; the prompts shrink.
- **Receiving** stays the pointer, then `relay_read`: the agent CLIs don't
  act on a server's notifications.
- The CLI stays for engines without MCP.

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
  Tracks draws in Settings under the plugin's name; the events it
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

## Order, once it's decided

1. Tracks' MCP server for what exists: the relay, the proxy, the panes.
2. The envelope's fields: `outcome`, `refs`.
3. The manifest and plugins; the Investigator and the Watcher as the
   first.
4. Programmable messages: deferred delivery and leases first, the two
   that pay off with side tracks, then the user's gates.

## Open

- Everything under "Plugins first", starting with A1 ("Planning: the
  agenda").
- How the track finds and starts agents, several at once ("Many
  instances").
- Contracts: YAML in a message, or a tool call (`relay_send` with a
  `contract` argument) that Tracks validates against the vocabulary?
- How a contract shows in the Relay Station: waiting, running, done,
  cancelled; and how the user cancels or approves one.
- Leases: enforced (Tracks refuses an agent's change to a leased file, as
  the worktree check catches edits) or advisory (a notice when broken)?
- How much of A2A (Google's open Agent2Agent protocol: tasks, their
  states, message parts) to borrow, while keeping the relay local and
  simple.
