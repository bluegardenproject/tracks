# Reviews

**Status:** being built, one PR per step under Order. Step 1 is built; the
reviewer moves into a pane of its own (see The reviewer pane).

## Problem

- **The reviewer is tied to the track's engine.** A Claude track reviews with
  the in-session `tracks-reviewer` subagent, on Claude's model. A second
  opinion from another engine and model (a Cursor reviewer on a Claude track,
  or the other way round) isn't possible.
- **A Review track's agent does everything.** It takes the user's brief,
  resolves links, runs the review, presents it and answers follow-ups, all on
  one model. The review needs the strongest model; the rest doesn't.
- **Reports flag extreme edge cases.** Warns, and even blocks, describe
  scenarios that need inputs or states the code around them rules out. They
  look as serious as real defects, and sorting them out is the user's work.
- **Large PRs.** In a big monorepo a PR can touch hundreds of files across
  several teams. The user is responsible for their team's part only.

## Not in scope

- **Posting to GitHub.** Reviews never comment on a PR. The user reads the
  report in Tracks and decides what to raise in the PR themselves. Posting
  would need agreement with the teams first, so PRs don't fill up with
  generated comments.
- **Cost of reviewer sessions.** Per-track cost is Claude-only and inaccurate,
  and may be removed; reviewer sessions don't add to it.

## What exists

| | Claude track | Cursor track |
|---|---|---|
| Pre-push review | `tracks-reviewer` subagent via the Task tool | `tracks review`: a headless `agent -p` with the diff |
| Review track | The same subagent | The same `tracks review` |
| Reviewer's model | The session's | `engines.cursor.model` |
| Instructions | `claude.ReviewerInstructions()` | The same text |
| Guards | — | `TRACKS_REVIEWING` marker and a per-track lock |

`tracks review` already holds most of what an engine-independent reviewer
needs: one set of instructions, the diff built by Tracks, the base from the
track's repo, the candor level, the guards.

## Roles

| Role | Model | Does |
|---|---|---|
| Orchestrator | The track's own, can be a smaller one | Takes the user's brief, resolves links it can open (a ticket, a doc), writes the brief, runs `tracks review`, presents the report, answers questions the report already answers |
| Reviewer | The Reviewer agent's (Settings → Agents) | Reviews in a session of its own, in the reviewer pane; answers questions about its review |

The orchestrator never reviews itself; when `tracks review` fails it says so
and stops. On a Work track the working agent is the orchestrator for its
pre-push review.

### What each may do

| | Edit files | Commit | Push, open a PR | Show a fix in the console |
|---|---|---|---|---|
| Reviewer, any track | never | never | never | in the report |
| Orchestrator, Review track | never | never | never | yes, as a suggestion |
| Orchestrator, Work track | yes | yes | after the pre-push review; a PR only once the user agreed, as a draft where the repo's Draft PRs setting says so | yes |

- **The reviewer can't change anything.** It runs in its engine's read-only
  mode (see The reviewer pane).
- **Tracks checks it.** `tracks review` notes the worktree's HEAD, `git
  status`, the changes to tracked files and the untracked files' contents
  before the reviewer starts, and compares them after. A change is
  reported above the findings, as an error, never quietly kept.
- **The orchestrator follows its track's kind.** The prompts carry the rules
  above: a Review track's ends with its read-only contract; a Work track's
  asks the user before opening a PR.

## Reviewer setting

- Settings tab, a **Review** section: engine (Claude, Cursor, or *same as the
  track*) and model, picked from what `agents/` reports for that engine.
- *Same as the track* is the default. With it, a Claude track keeps its
  subagent as today; any other choice makes it use `tracks review`. Once the
  headless Claude reviewer has proven itself, Claude tracks use `tracks review`
  always, and the subagent goes.
- A per-track override in the New track form comes later, if a global setting
  turns out not to be enough. It needs a migration.

## The reviewer pane

The reviewer is an interactive session of the Reviewer agent and model, in a
pane of its own, hidden until the user opens it.

- **Started by Tracks, not the orchestrator.** `tracks review` asks the daemon,
  which opens the pane through tmux. Its process is tmux's, not the
  orchestrator's: the orchestrator's sandbox doesn't apply, and no new way
  around it opens either. The reviewer runs as any track's agent does, with
  its engine's own permissions and sandbox for its commands.
- **Hidden.** Hidden panes live in a background tmux session of Tracks', a
  window per track, so they never show among the tracks.
- **Read-only.** Claude starts in plan mode with `Edit`, `Write` and
  `NotebookEdit` denied; Cursor with `--mode ask`. `tracks review` also
  checks the worktree before and after, in the orchestrator's own sandbox.
- **The report comes back through the engine's hooks**, which Tracks already
  installs: when Claude stops, its last reply is read from the transcript;
  for Cursor, the hook event that carries the reply's text. `tracks review`
  waits for it and prints it to the orchestrator. The reviewer never has to
  run a command.
- **In Work tracks too.** The pane opens in the Work track's window and
  worktree, so a pre-push review sees what isn't committed yet. The pane
  route replaces the in-session `tracks-reviewer` subagent once it's built.

### The button

While a track has a reviewer, the footer's top row shows a green
**Open reviewer** button. It moves the hidden pane into the track's right
column, as a dev-server pane sits, running on as it was (`join-pane`); the
button becomes **Hide reviewer**, which moves it back. There's no button to
end it.

Two button variants are added. They aren't part of the rotation the footer's
other buttons take (variants 1 to 4); each belongs to one button:

| Variant | Use | Default (dark) | Default light |
|---|---|---|---|
| `button.variant5` | Reserved: an End reviewer option, or the Station | bg `#700c50`, text `#ffb5e7` | bg `#ffd9f1`, text `#700c50` |
| `button.variant6` | Open reviewer / Hide reviewer | bg `#00916f`, text `#c9fff0` | bg `#c9fff0`, text `#00664f` |

### Lifetime

- **One reviewer per track.** It stays, hidden or shown, until the track ends.
- **A new review starts fresh.** `tracks review` ends a reviewer that's there
  and starts a new session, after the branch changed or for another scope.
- **Closing its pane ends it**, such as the orchestrator running
  `tracks close` when the user asks.
- **Interaction is limited.** The reviewer answers questions about its review
  and the code it reviewed. It takes no new tasks, doesn't plan or build, and
  points anything else back to the orchestrator: it is not a second track.

### Why not the earlier routes

- **Headless from the orchestrator's shell** (built in step 1): a Claude
  track's sandbox keeps the reviewer's CLI from its network.
- **Headless from the daemon:** the daemon runs outside any sandbox, so the
  reviewer did too. Claude's could be locked down with flags; Cursor's needed
  an OS sandbox and still loaded hooks and config from the worktree, which the
  agent can write. Dropped.

## The review packet

Tracks builds the packet; the orchestrator adds only what needs judgment or a
connection Tracks doesn't have. Code is always local: a Review track's
worktree is detached at the PR head or the branch tip, so GitHub adds what's
around the code, never the code.

| Part | Pre-push | PR review | From |
|---|---|---|---|
| Diff | `base...HEAD`, and what isn't committed | `origin/<PR base>...HEAD` | git |
| Changed files | Paths with added and removed lines | The same | git |
| Intent | Commit messages | PR title and body | git, `gh` |
| Discussion | — | Open review threads, so findings aren't repeated and addressed ones are checked | `gh` |
| Checks | — | CI results; a failing check is a finding, its log the evidence | `gh pr checks` |
| Brief | The user's words | The form's prompt, and what the orchestrator resolved | `--brief <file>` |
| Settings | Candor, base, scope | The same | Tracks |

- **A moved PR.** Before a PR review, Tracks compares the worktree's HEAD with
  the PR's head SHA. When the PR moved on, it says so and asks whether to
  fetch; it never reviews a stale head silently.
- **Limits.** The changed-file list is always complete. Diffs over the limit
  aren't cut in the middle of a file: whole files are left out, the largest
  first, and listed as not reviewed in the report's Not run line. Threads and
  CI logs get their own limits.

## Checks

Review tracks don't run lint, typecheck or tests locally. A PR review relies on
the CI results in the packet; a failing check is a finding. A Review track's
setup stays lazy, as now.

## Scope: a code owner's part

`tracks review --owner <handle>`, and a **Code owner** field per repo on the
Repositories tab that sets it by default (empty: the whole diff).

- Tracks reads the repo's `CODEOWNERS` (`.github/`, the root, `docs/`, the
  first found) at the reviewed head, with GitHub's rules: the last matching
  pattern wins. Parsing and matching are a pure function with tests.
- The packet's diff holds only the files `<handle>` owns. The reviewer may
  still read any file for context, such as a caller outside the scope.
- The report's first line says the scope: `Reviewed 38 of 412 changed files,
  those owned by <handle>.` A change outside the scope that breaks owned code
  is still reported, under `### Whole diff`.
- No `CODEOWNERS`, or no file owned by the handle: the review stops and says
  so, rather than reviewing everything.

## Likelihood and confidence

Each finding gets two numbers next to its severity:

```
- 🔴 **block** · `L42` · 90% sure · 60% likely — <one-sentence finding>
  > Why: …
  > Path: <the caller chain or input that gets there>
```

- **Confidence** (*sure*): how certain the reviewer is that the defect is
  real. It is high when the reviewer read the code involved and traced the
  path; low when the finding rests on code it didn't open or behaviour it
  assumes.
- **Likelihood** (*likely*): how probable the failure scenario is in real use,
  given the callers and data that exist today. The reviewer anchors it:

  | Likely | Means |
  |---|---|
  | 70–100% | A normal path: common inputs, every run, every user |
  | 30–70% | Realistic conditions: a slow network, an empty list, a retry |
  | 5–30% | Valid but unusual inputs or timing |
  | 0–5% | Needs a state the callers or types rule out today |

- **Path.** A block or warn names how the scenario is reached: the call chain
  or input. No path found: likelihood 20% at most, and the report says the
  path wasn't found.
- Severity stays what it is now, how bad the failure is when it happens. The
  numbers sit beside it, so the user can sort out edge cases at a glance.

### Experiments

How well the numbers work is measured, not assumed. In order:

1. **Report only.** The numbers in the report; severity unchanged. The user
   sees whether they match their judgement.
2. **A verify pass.** Each block and warn goes to a second reviewer (another
   model, or the same with a fresh context) told to refute it. A finding it
   refutes is dropped; one it confirms keeps its confidence. Costs a call per
   finding; on for blocks only at first.
3. **Gating.** If the numbers prove reliable, severity follows them: a block
   under 10% likely becomes a warn, a warn under 5% a hint. Thresholds come
   from step 4.
4. **Calibration.** In Tracks the user marks a finding *raised* (flagged in the
   PR) or *dismissed*. Stored per finding with its numbers, which shows over
   time whether 60% likely means anything. Needs the findings stored, so the
   report becomes structured data (JSON next to the text).

## Follow-ups

The orchestrator answers what the report already holds: "summarise the
blocks", "which findings touch the cache?", "explain finding 3". It doesn't
add findings or change their severity, likelihood or confidence.

Everything that needs the code read again goes back to the reviewer:

- **A question about a finding** ("is finding 3 a false positive?", "look
  deeper at the cache"): the user asks in the reviewer pane, or the
  orchestrator runs `tracks review --ask "<question>"`, which types it into
  the reviewer's session and returns its answer.
- **A new review**, after the branch changed, or **another scope** (another
  `--owner`, the whole diff): a new `tracks review`, a new session.

## Control surface

| Command | Does |
|---|---|
| `tracks review` | Starts a reviewer session on the track's branch, ending one that's there, and prints its report |
| `--brief <file>` | Adds the orchestrator's brief to the packet |
| `--owner <handle>` | Reviews only the files `<handle>` owns |
| `--ask <question>` | Asks the reviewer that's there, and prints its answer |
| `--base <ref>` | As now |

## Order

Each step is a PR of its own, branched from `main` where it can be, so they
can be reviewed and merged while the next is built.


1. `tracks review` engine-independent, and the Claude reviewer's flags.
   Built.
2. Settings → Agents → Reviewer: the reviewer's agent and model.
3. The reviewer pane: the hidden pane, read-only modes, the report through
   the hooks, `tracks review` waiting for it, a new review ending the old
   session.
4. The footer button, Open and Hide reviewer, with `button.variant6`;
   `button.variant5` added and reserved.
5. Orchestrators use the pane: Work and Review tracks, both engines; the
   `tracks-reviewer` subagent goes.
6. `tracks review --ask`.
7. The packet: changed files, PR intent, threads, checks, the moved-PR check,
   per-file limits.
8. Code-owner scope: `CODEOWNERS` matching, `--owner`, the repo field.
9. Likelihood and confidence in the report (experiment 1).
10. The verify pass (experiment 2).
11. Structured findings and calibration (experiments 3 and 4).

## Open

- Which of Cursor's hook events carries the reply's text, for the report.
- Where calibration data is shown: per engine and model, to compare reviewers.
