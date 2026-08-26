package settings

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
)

func TestModelSummaryReportsWhatIsConfigured(t *testing.T) {
	cases := []struct {
		name   string
		claude config.Claude
		want   string
	}{
		{"nothing set", config.Claude{}, "Claude's own"},
		{"default only", config.Claude{Model: "claude-opus-4-8"}, "claude-opus-4-8"},
		{
			"default plus overrides",
			config.Claude{Model: "opus", ModelByKind: map[string]string{"doc": "haiku"}},
			"opus, 1 per-kind",
		},
		// Overrides with no global default still have to be reported —
		// they are the thing most easily forgotten about, and hiding
		// them behind an unset default makes the menu line a lie.
		{
			"overrides only",
			config.Claude{ModelByKind: map[string]string{"doc": "haiku", "ask": "sonnet"}},
			"Claude's own, 2 per-kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := modelSummary(config.Config{Claude: tc.claude}); got != tc.want {
				t.Errorf("modelSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

// A model set by hand in the YAML that isn't one of the picker choices
// must survive opening the form — otherwise the select would have no
// matching option and silently reset it on save.
func TestModelOptionsKeepAnUnlistedCurrentValue(t *testing.T) {
	cfg := config.Config{Claude: config.Claude{
		ModelChoices: []config.ModelChoice{{Label: "Opus", Model: "opus"}},
	}}
	opts := modelOptions(cfg, "claude-opus-4-7", "none")

	var found bool
	for _, o := range opts {
		if o.Value == "claude-opus-4-7" {
			found = true
			if !strings.Contains(o.Key, "current") {
				t.Errorf("unlisted current value not marked: %q", o.Key)
			}
		}
	}
	if !found {
		t.Errorf("hand-configured model missing from the options: %+v", opts)
	}
}

// The empty option has to come first so "leave it alone" is the
// no-keystroke answer.
func TestModelOptionsLeadWithTheEmptyChoice(t *testing.T) {
	opts := modelOptions(config.Config{}, "", "Claude's own default")
	if len(opts) == 0 {
		t.Fatal("no options")
	}
	if opts[0].Value != "" {
		t.Errorf("first option is %q, want the empty choice", opts[0].Value)
	}
}

// The built-in choices are implicit — the config carries an empty list.
// Appending to that list without materialising it first would save a
// one-entry list and silently drop the other built-ins.
func TestAddingAChoiceMaterialisesTheBuiltInList(t *testing.T) {
	cfg := config.Config{}
	builtIn := len(config.DefaultModelChoices())

	appendModelChoice(&cfg, "Opus 4.8", "claude-opus-4-8")

	if got := len(cfg.Claude.ModelChoices); got != builtIn+1 {
		t.Errorf("choices = %d, want the %d built-ins plus the new one", got, builtIn)
	}
	if got := cfg.Claude.Choices(); len(got) != builtIn+1 {
		t.Errorf("Choices() = %d entries, want %d", len(got), builtIn+1)
	}

	// Adding the same model twice must not duplicate it.
	appendModelChoice(&cfg, "Opus 4.8 again", "claude-opus-4-8")
	if got := len(cfg.Claude.ModelChoices); got != builtIn+1 {
		t.Errorf("choices = %d after re-adding the same model, want %d", got, builtIn+1)
	}
}

// Removing from the implicit built-in list must leave the rest of it
// behind rather than clearing back to the defaults.
func TestRemovingAChoiceMaterialisesTheBuiltInList(t *testing.T) {
	cfg := config.Config{}
	choices := cfg.Claude.Choices()
	builtIn := len(choices)
	dropped := choices[0].Model

	removeModelChoiceAt(&cfg, choices, 0)

	if got := len(cfg.Claude.ModelChoices); got != builtIn-1 {
		t.Fatalf("choices = %d after removing one, want %d", got, builtIn-1)
	}
	for _, c := range cfg.Claude.ModelChoices {
		if c.Model == dropped {
			t.Errorf("%q survived removal", dropped)
		}
	}
}

// The per-kind form edits the overrides, so showing their count inside
// it reads as if the default were several models at once.
func TestDefaultModelLabelOmitsThePerKindCount(t *testing.T) {
	cfg := config.Config{Claude: config.Claude{
		Model:       "opus",
		ModelByKind: map[string]string{"doc": "haiku", "ask": "sonnet"},
	}}
	if got := defaultModelLabel(cfg); got != "opus" {
		t.Errorf("defaultModelLabel = %q, want just the global default", got)
	}
	if got := modelSummary(cfg); got == defaultModelLabel(cfg) {
		t.Error("modelSummary should still report the per-kind count")
	}
}

// An empty choices list means "use the built-ins", so emptying a
// curated list would silently restore all four aliases instead of
// leaving none — and omitempty means the YAML wouldn't show why.
func TestTheLastChoiceCannotBeRemoved(t *testing.T) {
	cfg := config.Config{Claude: config.Claude{
		ModelChoices: []config.ModelChoice{{Label: "Opus 4.8", Model: "claude-opus-4-8"}},
	}}
	if removeModelChoiceAt(&cfg, cfg.Claude.Choices(), 0) {
		t.Error("removing the last choice reported success")
	}
	got := cfg.Claude.Choices()
	if len(got) != 1 || got[0].Model != "claude-opus-4-8" {
		t.Errorf("choices = %+v, want the curated single entry intact rather than the built-ins back", got)
	}
}

// The per-kind count belongs on the overrides row only. It first
// shipped on the "Default model" row too, where it reads as though the
// default were several models at once.
func TestModelMenuPutsThePerKindCountOnOneRowOnly(t *testing.T) {
	cfg := config.Config{Claude: config.Claude{
		Model:       "opus",
		ModelByKind: map[string]string{"doc": "haiku", "ask": "sonnet"},
	}}

	var withCount []string
	for _, o := range modelMenuOptions(cfg) {
		if strings.Contains(o.Key, "per-kind") || strings.Contains(o.Key, "2 set") {
			withCount = append(withCount, o.Key)
		}
	}
	if len(withCount) != 1 {
		t.Errorf("expected exactly one row to carry the override count, got %d: %q", len(withCount), withCount)
	}
	if len(withCount) == 1 && !strings.HasPrefix(withCount[0], "Per-kind overrides") {
		t.Errorf("the count landed on %q, want it on the Per-kind overrides row", withCount[0])
	}
}

// The top-level row is the first thing a user reads, and it carries the
// same trap: calling the summary "the default" turns a count of
// overrides into an apparent list of default models.
func TestModelsMenuLabelDoesNotCallTheSummaryADefault(t *testing.T) {
	cfg := config.Config{Claude: config.Claude{
		Model:       "opus",
		ModelByKind: map[string]string{"doc": "haiku", "ask": "sonnet"},
	}}
	got := modelsMenuLabel(cfg)
	if strings.Contains(got, "default:") {
		t.Errorf("label %q presents the whole summary as the default", got)
	}
	if !strings.Contains(got, "2 per-kind") {
		t.Errorf("label %q dropped the override count", got)
	}
}
