package settings

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/tui"
	"github.com/charmbracelet/huh"
)

// modelsMenuLabel is the Models row on the top-level settings menu.
//
// No "default:" prefix: modelSummary describes the whole model config,
// overrides included, so labelling it as the default renders
// "default: opus, 2 per-kind" — the same "the default is several
// models" misreading the submenu was fixed for.
func modelsMenuLabel(cfg config.Config) string {
	return fmt.Sprintf("Models  (%s)", modelSummary(cfg))
}

// modelMenuOptions builds the Models submenu rows. Extracted from the
// form so the labels are reachable from a test: the per-kind count
// belongs on the overrides row and nowhere else, and that had already
// been got wrong once on the row above it.
func modelMenuOptions(cfg config.Config) []huh.Option[modelAction] {
	return []huh.Option[modelAction]{
		huh.NewOption(fmt.Sprintf("Default model  (%s)", defaultModelLabel(cfg)), modelActionDefault),
		huh.NewOption(fmt.Sprintf("Per-kind overrides  (%d set)", len(cfg.Claude.ModelByKind)), modelActionPerKind),
		huh.NewOption(fmt.Sprintf("Picker choices  (%d)", len(cfg.Claude.Choices())), modelActionChoices),
		huh.NewOption("Back", modelActionBack),
	}
}

// defaultModelLabel names the global default alone — no per-kind
// count. Used inside the per-kind form, where the count is both
// meaningless and actively confusing ("the default is two models?").
func defaultModelLabel(cfg config.Config) string {
	if m := strings.TrimSpace(cfg.Claude.Model); m != "" {
		return m
	}
	return "Claude's own"
}

// modelSummary is the one-line state shown on the settings menu, so
// what's configured is visible without opening the section. The
// per-kind count is reported whether or not a global default is set:
// overrides are the thing most easily forgotten about.
func modelSummary(cfg config.Config) string {
	label := defaultModelLabel(cfg)
	if n := len(cfg.Claude.ModelByKind); n > 0 {
		return fmt.Sprintf("%s, %d per-kind", label, n)
	}
	return label
}

type modelAction string

const (
	modelActionDefault modelAction = "default"
	modelActionPerKind modelAction = "perkind"
	modelActionChoices modelAction = "choices"
	modelActionBack    modelAction = "back"
)

// editModels is the Models section: the default a new track runs, the
// per-kind overrides, and the list the creation picker offers. Saves
// after each mutation, like editRepo.
func editModels(cfg *config.Config) error {
	for {
		var pick modelAction
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[modelAction]().
					Title("Models").
					Description("Which model a new track runs, and what the creation picker offers.").
					Options(modelMenuOptions(*cfg)...).
					Value(&pick),
			),
		)
		if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch pick {
		case modelActionBack:
			return nil
		case modelActionDefault:
			if err := editDefaultModel(cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		case modelActionPerKind:
			if err := editModelsByKind(cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		case modelActionChoices:
			if err := editModelChoices(cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		}
		if _, err := config.Save(*cfg); err != nil {
			fmt.Println("save failed:", err)
			waitForKey()
		}
	}
}

// modelOptions is the picker list plus the two ends of the range: an
// empty choice meaning "leave it to Claude", and whatever is currently
// configured if it isn't already one of the choices (so opening the
// form doesn't silently discard a hand-edited value).
func modelOptions(cfg config.Config, current, emptyLabel string) []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption(emptyLabel, "")}
	seen := map[string]bool{}
	for _, c := range cfg.Claude.Choices() {
		label := strings.TrimSpace(c.Label)
		if label == "" {
			label = c.Model
		}
		opts = append(opts, huh.NewOption(label, c.Model))
		seen[c.Model] = true
	}
	if current = strings.TrimSpace(current); current != "" && !seen[current] {
		opts = append(opts, huh.NewOption(current+"  (current)", current))
	}
	return opts
}

func editDefaultModel(cfg *config.Config) error {
	model := cfg.Claude.Model
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default model").
				Description("What a new track runs when its kind has no override and nothing is picked at creation.").
				Options(modelOptions(*cfg, model, "Claude's own default (pass no --model)")...).
				Value(&model),
		),
	)
	if err := runSettingsForm(form); err != nil {
		return err
	}
	cfg.Claude.Model = strings.TrimSpace(model)
	return nil
}

func editModelsByKind(cfg *config.Config) error {
	// Copy so an aborted form leaves the config untouched.
	byKind := map[string]string{}
	for k, v := range cfg.Claude.ModelByKind {
		byKind[k] = v
	}

	// config.ModelKinds is the list Validate enforces and that
	// TestConfigModelKindsMatchStateKinds pins against state.Kind. A
	// local copy here would let a new kind quietly never appear in this
	// form.
	kinds := config.ModelKinds()
	values := make([]string, len(kinds))
	fields := make([]huh.Field, 0, len(kinds))
	for i, kind := range kinds {
		values[i] = byKind[kind]
		fields = append(fields, huh.NewSelect[string]().
			Title(strings.ToUpper(kind[:1])+kind[1:]).
			Options(modelOptions(*cfg, values[i], fmt.Sprintf("Use the default (%s)", defaultModelLabel(*cfg)))...).
			Value(&values[i]))
	}

	if err := runSettingsForm(huh.NewForm(huh.NewGroup(fields...))); err != nil {
		return err
	}

	for i, kind := range kinds {
		if v := strings.TrimSpace(values[i]); v != "" {
			byKind[kind] = v
		} else {
			delete(byKind, kind)
		}
	}
	// Drop the key entirely when nothing is set, so the saved YAML stays
	// clean rather than carrying an empty map.
	if len(byKind) == 0 {
		cfg.Claude.ModelByKind = nil
	} else {
		cfg.Claude.ModelByKind = byKind
	}
	return nil
}

type choiceAction string

const (
	choiceAdd    choiceAction = "add"
	choiceRemove choiceAction = "remove"
	choiceReset  choiceAction = "reset"
	choiceBack   choiceAction = "back"
)

func editModelChoices(cfg *config.Config) error {
	for {
		listed := make([]string, 0, len(cfg.Claude.Choices()))
		for _, c := range cfg.Claude.Choices() {
			label := strings.TrimSpace(c.Label)
			if label == "" || label == c.Model {
				listed = append(listed, c.Model)
				continue
			}
			listed = append(listed, fmt.Sprintf("%s → %s", label, c.Model))
		}
		desc := "Offered by the model picker when a track is created.\n  " + strings.Join(listed, "\n  ")
		if len(cfg.Claude.ModelChoices) == 0 {
			desc += "\n\n(built-in list — aliases follow each family's newest release)"
		}

		var pick choiceAction
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[choiceAction]().
					Title("Picker choices").
					Description(desc).
					Options(
						huh.NewOption("Add a model", choiceAdd),
						huh.NewOption("Remove a model", choiceRemove),
						huh.NewOption("Reset to the built-in list", choiceReset),
						huh.NewOption("Back", choiceBack),
					).
					Value(&pick),
			),
		)
		if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch pick {
		case choiceBack:
			return nil
		case choiceReset:
			cfg.Claude.ModelChoices = nil
		case choiceAdd:
			if err := addModelChoice(cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		case choiceRemove:
			if err := removeModelChoice(cfg); err != nil && !errors.Is(err, ErrCancelled) {
				return err
			}
		}
		if _, err := config.Save(*cfg); err != nil {
			fmt.Println("save failed:", err)
			waitForKey()
		}
	}
}

func addModelChoice(cfg *config.Config) error {
	var model, label string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Model").
				Description("An alias (opus, sonnet, haiku, fable), which follows that family's newest release, or a pinned id (claude-opus-4-8), which doesn't.\n\nNote: the CLI does not reliably reject a mistyped name — \"opus4.8\" runs on a different model rather than failing. The dashboard's MODEL column shows what actually ran.").
				Placeholder("claude-opus-4-8").
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return errors.New("a model is required")
					}
					if strings.ContainsAny(v, " \t") {
						return errors.New("a model id has no spaces — check for a typo")
					}
					return nil
				}).
				Value(&model),
			huh.NewInput().
				Title("Label (optional)").
				Description("What the picker shows. Empty uses the model itself.").
				Placeholder("Opus 4.8").
				Value(&label),
		),
	)
	if err := runSettingsForm(form); err != nil {
		return err
	}

	appendModelChoice(cfg, label, model)
	return nil
}

// appendModelChoice adds one entry to the picker list, ignoring a model
// already offered. Split out from the form so the list logic is
// reachable without a TTY.
//
// It materialises the built-in list first: the choices are implicit
// while the config carries none, so appending straight to the empty
// slice would save a one-entry list and silently drop the built-ins.
func appendModelChoice(cfg *config.Config, label, model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	choices := cfg.Claude.ModelChoices
	if len(choices) == 0 {
		choices = config.DefaultModelChoices()
	}
	for _, c := range choices {
		if c.Model == model {
			fmt.Printf("%s is already offered; nothing added.\n", model)
			waitForKey()
			return
		}
	}
	cfg.Claude.ModelChoices = append(choices, config.ModelChoice{
		Label: strings.TrimSpace(label),
		Model: model,
	})
}

// removeModelChoiceAt drops the entry at idx, materialising the
// built-in list first for the same reason as appendModelChoice.
// Reports false when nothing was removed.
//
// The last entry cannot be removed: an empty list is how "use the
// built-ins" is expressed, so emptying a curated list would silently
// restore all four aliases rather than leaving none — and omitempty
// means the YAML wouldn't show why.
func removeModelChoiceAt(cfg *config.Config, choices []config.ModelChoice, idx int) bool {
	if idx < 0 || idx >= len(choices) || len(choices) == 1 {
		return false
	}
	cfg.Claude.ModelChoices = append(append([]config.ModelChoice{}, choices[:idx]...), choices[idx+1:]...)
	return true
}

func removeModelChoice(cfg *config.Config) error {
	choices := cfg.Claude.ModelChoices
	if len(choices) == 0 {
		choices = config.DefaultModelChoices()
	}
	options := make([]huh.Option[int], 0, len(choices))
	for i, c := range choices {
		label := strings.TrimSpace(c.Label)
		if label == "" {
			label = c.Model
		}
		options = append(options, huh.NewOption(fmt.Sprintf("%s  (%s)", label, c.Model), i))
	}

	idx := -1
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Remove which model?").
				Options(options...).
				Value(&idx),
		),
	)
	if err := runSettingsForm(form); err != nil {
		return err
	}
	if !removeModelChoiceAt(cfg, choices, idx) {
		fmt.Println("can't remove the last choice — the picker needs at least one. Use \"Reset to the built-in list\" instead.")
		waitForKey()
	}
	return nil
}

// runSettingsForm runs a form with the shared keymap, translating an
// abort into ErrCancelled so callers can treat it as "left unchanged".
func runSettingsForm(form *huh.Form) error {
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}
	return nil
}
