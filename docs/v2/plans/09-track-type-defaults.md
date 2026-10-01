# Plan: default agent and model per track type

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunks 5 (real tracks) and 7 (Tracks window content). Each track type, Work, Ask, Plan, Review and Doc, gets its own default agent and model, set in Settings. New tracks run on their type's.

## What users get

- **Settings → Tracks:** the **Track** placeholder becomes **Tracks**, with one part for now, **Track type default models**:
  - A row of the types, **Work  Ask  Plan  Review  Doc**. Clicking one, or ←/→, selects it.
  - Below it, the selected type's two fields, like the theme field: **Agent** (`[ Claude Code ▾ ]`) and **Model** (`[ Default (opus) ▾ ]`). A click, or Enter on the focused field, opens a picker. ↑/↓ and Tab move between the types' row and the fields.
  - **The Agent picker** lists the agents Tracks knows, **Claude Code** and **Cursor**. One that isn't added on the Engines tab says so ("not added").
  - **The Model picker** is the Engines tab's: its first entry is **Default**, the agent's default model from the Engines tab ("Claude Code's default: opus", or "Claude Code chooses" when that's the CLI's own). Then Claude Code's list from the Engines tab, the four aliases plus the models the user added there, or Cursor's `agent --list-models`. The Engines tab stays the one place that lists models.
  - Picking saves at once, like the Engines tab. Picking another agent sets the model back to **Default**, since models belong to their agent.
  - When the agent isn't added, the fields say so below them: "Cursor isn't added on the Engines tab, so Work tracks can't start."
- **A type the user hasn't set follows the Engines tab:** Claude Code with its default model, or Cursor when only Cursor is added, as tracks run today. Changing Claude Code's default model on the Engines tab changes every such type.
- **Create** runs a track on its type's agent and model. A type set to an agent that isn't added refuses: "Add Cursor on the Engines tab, or pick another agent for Work tracks in Settings → Tracks." With no agent added at all, it says "Add an engine on the Engines tab first." as today.
- **The New track form** shows what the chosen type runs on, above its buttons ("Runs on Claude Code, model opus.", or "Runs on Cursor, which isn't added on the Engines tab."), and updates when the type changes. The form's **Select engine** field, built since, below Name, lets the user pick another agent or model for one track; see [06-add-track.md](06-add-track.md).
- **Resume** is unchanged: a track keeps the agent and model it was created with.

## Settings

A `tracks` section in `~/.config/tracks-v2/settings.yaml`, one key per type the user set. Keys this build doesn't know stay, as with `engines`.

```yaml
tracks:
  work:
    engine: claude
    model: opus     # "" or missing: the agent's default from the Engines tab
  ask:
    engine: cursor
```

- A type's entry always names its agent; one without an entry follows the Engines tab.
- The model is stored as picked, so a model later removed from the Engines tab list is still passed as is. The Model field shows it with "not listed on the Engines tab".

## How a track's agent and model are chosen

`settings` answers it, so Create, the form and the Settings section agree:

1. The type's entry names the agent: that one, added or not.
2. Otherwise Claude Code when it's added, else Cursor (today's rule).
3. The model is the entry's, when it has one; otherwise the agent's default model from the Engines tab, which may be "" for the CLI's own.

## Packages

No new packages. Changed:
- `settings`: `Tracks` (the `tracks` section, by type) and `RunsOn(kind)`, the rule above.
- `tracks`: Create uses `RunsOn`, and refuses an agent that isn't added.
- `ui/source`: a `TrackTypes` source that loads and saves the `tracks` section; `cli` implements it on `settings.yaml`.
- `ui/tracksview`: the Tracks section; the model picker's items are shared with the Engines tab.
- `ui/addtrack` and `cli`: the form gets what each type runs on, instead of one engine and model.

## Not in this plan

- Choosing the agent or model in the New track form, for one track.
- Other defaults per type (auto mode, a terminal pane): the section can grow into them.
- Fast Tracks.

## Tests

- **`settings`:** `RunsOn` for a set type, an unset one with Claude added, with only Cursor, with none, and a set agent that isn't added; saving `tracks` keeps keys this build doesn't know.
- **`tracks`:** Create runs on the type's agent and model, and refuses a set agent that isn't added.
- **`ui/tracksview`:** selecting a type shows its fields; picking an agent resets the model and saves; the Model picker lists Claude Code's added models and the Default entry with the Engines tab's model; the "isn't added" line.
- **`ui/addtrack`:** the "Runs on" line follows the chosen type.

## Delivery

On the branch with the notice fix, `feat/v2-track-defaults`, one PR: this plan; `settings`; `tracks`; the Tracks section; the form; docs.
