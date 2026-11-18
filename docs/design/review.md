# Reviews

**Status:** proposed. Nothing here is built yet; the steps under Order are one
PR each.

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
| Reviewer | The review setting's engine and model | Reads the packet and the code around it, writes the report |

The orchestrator never reviews itself; when `tracks review` fails it says so
and stops. On a Work track the working agent is the orchestrator for its
pre-push review.

### What each may do

| | Edit files | Commit | Push, open a PR | Show a fix in the console |
|---|---|---|---|---|
| Reviewer, any track | never | never | never | in the report |
| Orchestrator, Review track | never | never | never | yes, as a suggestion |
| Orchestrator, Work track | yes | yes | after the pre-push review; a PR only once the user agreed, as a draft where the repo's Draft PRs setting says so | yes |

- **The reviewer can't change anything.** The Claude reviewer gets read-only
  tools only (below), with `Edit`, `Write`, `Bash(git commit:*)` and
  `Bash(git push:*)` denied as well. The Cursor reviewer runs without
  permission flags, so it can't run commands at all.
- **Tracks checks it.** `tracks review` notes the worktree's HEAD and its
  `git status` before the reviewer starts and compares them after. A change is
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

### The Claude reviewer

`claude -p` with the reviewer instructions as its system prompt, the packet on
stdin, `--model` from the setting and tools limited to reading: `Read`,
`Grep`, `Glob`, and `Bash(git diff:*)`, `Bash(git log:*)`, `Bash(git show:*)`.
The marker and the lock move from `cursor.Review` into the shared
`tracks review`, so both engines have them.

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

Everything that needs the code read again goes back to a reviewer:

- **A question about a finding** ("is finding 3 a false positive?", "look
  deeper at the cache"): `tracks review --follow-up "<question>"` resumes the
  last reviewer session (`claude --resume`, Cursor's `--resume`), so the
  reviewer answers with its context.
- **A new review**, after the branch changed, or **another scope** (another
  `--owner`, the whole diff): a new `tracks review`, a new session.

## Control surface

| Command | Does |
|---|---|
| `tracks review` | Reviews the track's branch with the review setting's engine and model |
| `--brief <file>` | Adds the orchestrator's brief to the packet |
| `--owner <handle>` | Reviews only the files `<handle>` owns |
| `--follow-up <question>` | Asks the last reviewer session again |
| `--base <ref>` | As now |

## Order

Each step is a PR of its own, branched from `main` where it can be, so they
can be reviewed and merged while the next is built.


1. `tracks review` engine-independent: the Claude backend, guards in the shared
   path, the review setting in `settings.yaml`.
2. The Review section on the Settings tab.
3. The packet: changed files, PR intent, threads, checks, the moved-PR check,
   per-file limits.
4. Code-owner scope: `CODEOWNERS` matching, `--owner`, the repo field.
5. Likelihood and confidence in the report (experiment 1).
6. Review tracks as orchestrator and reviewer; Claude tracks always use
   `tracks review`.
7. Follow-ups.
8. The verify pass (experiment 2).
9. Structured findings and calibration (experiments 3 and 4).

## Open

- The reviewer's live view: a headless reviewer shows only its report. A pane
  that streams its progress, like a setup pane, may be worth it.
- Where calibration data is shown: per engine and model, to compare reviewers.
