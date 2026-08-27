package newtrack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/huh"
)

// Real output from `agent --list-models`, warts included: a header
// line, a "(current, default)" suffix, and the certificate warnings
// the CLI prints on macOS.
const realListModels = `ERROR: failed to copy trust settings of system certificate-25291
Available models

auto - Auto (current, default)
gpt-5.3-codex-low - Codex 5.3 Low
claude-opus-5-thinking-high - Claude Opus 5 1M Thinking
cursor-grok-4.6-low-fast - Cursor Grok 4.6 Low Fast
composer-2.5 - Composer 2.5
`

func TestParseModelListOnRealOutput(t *testing.T) {
	got := parseModelList(realListModels)
	if len(got) != 5 {
		t.Fatalf("parsed %d models, want 5: %+v", len(got), got)
	}
	if got[0].Model != "auto" || got[0].Label != "Auto (current, default)" {
		t.Errorf("first entry = %+v", got[0])
	}
	for _, m := range got {
		if m.Model == "" || m.Label == "" {
			t.Errorf("entry with an empty half: %+v", m)
		}
	}
}

// The header and the warning lines must not become models — an id is
// a single token, and prose that happens to contain " - " is not one.
func TestParseModelListRejectsProse(t *testing.T) {
	for name, in := range map[string]string{
		"header only":  "Available models\n",
		"prose dash":   "Please run agent login - then try again\n",
		"empty":        "",
		"no separator": "gpt-5.3-codex\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := parseModelList(in); len(got) != 0 {
				t.Errorf("parsed %+v from %q", got, in)
			}
		})
	}
}

// Cursor is only offered when its binary is actually there; offering
// it and failing at spawn is worse than not offering it.
func TestProviderChoicesFollowInstalledBinaries(t *testing.T) {
	cfg := config.Default()
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "not-installed")
	if got := providerChoices(cfg); len(got) != 1 || got[0] != state.ProviderClaude {
		t.Errorf("with no Cursor installed, choices = %v, want [claude]", got)
	}

	fake := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Cursor.Binary = fake
	if got := providerChoices(cfg); len(got) != 2 {
		t.Errorf("with Cursor installed, choices = %v, want both", got)
	}
}

// A Claude-only install must look exactly as it did before Cursor
// support existed — no one-option select on a form filled in daily.
func TestProviderPickerHiddenWhenThereIsNoChoice(t *testing.T) {
	cfg := config.Default()
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "not-installed")
	var v string
	if got := providerFields(cfg, &v); len(got) != 0 {
		t.Errorf("picker shown with only one provider available")
	}
}

// A default naming a provider that isn't installed must not preselect
// something the picker cannot offer.
func TestDefaultProviderFallsBackWhenUnavailable(t *testing.T) {
	cfg := config.Default()
	cfg.Provider = "cursor"
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "not-installed")
	if got := defaultProvider(cfg); got != string(state.ProviderClaude) {
		t.Errorf("defaultProvider = %q, want claude when cursor is not installed", got)
	}
}

// Claude's list comes from config; Cursor's does not, because only the
// binary knows what the account can reach.
func TestModelChoicesComeFromTheRightPlace(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.ModelChoices = []config.ModelChoice{{Label: "Pinned", Model: "claude-opus-4-8"}}
	cfg.Cursor.ModelChoices = []config.ModelChoice{{Label: "Codex", Model: "gpt-5.3-codex"}}

	if got := modelChoicesFor(cfg, "claude"); len(got) != 1 || got[0].Model != "claude-opus-4-8" {
		t.Errorf("claude choices = %+v", got)
	}
	if got := modelChoicesFor(cfg, "cursor"); len(got) != 1 || got[0].Model != "gpt-5.3-codex" {
		t.Errorf("cursor choices = %+v", got)
	}
	// An empty provider is Claude, per state.Provider.Resolved.
	if got := modelChoicesFor(cfg, ""); len(got) != 1 || got[0].Model != "claude-opus-4-8" {
		t.Errorf("empty provider did not fall back to claude: %+v", got)
	}
}

// When the CLI can't be asked, the picker degrades to "Default" only —
// which passes no --model. Never a wrong model, just an unchosen one.
func TestCursorChoicesDegradeWhenTheCLIFails(t *testing.T) {
	cfg := config.Default()
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "not-installed")
	if got := modelChoicesFor(cfg, "cursor"); len(got) != 0 {
		t.Errorf("expected no choices when the CLI cannot be reached, got %+v", got)
	}
}

func TestDefaultModelForUsesThePerProviderConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.Model = "opus"
	cfg.Cursor.Model = "gpt-5.3-codex"
	if got := defaultModelFor(cfg, "claude", "work"); got != "opus" {
		t.Errorf("claude default = %q", got)
	}
	if got := defaultModelFor(cfg, "cursor", "work"); got != "gpt-5.3-codex" {
		t.Errorf("cursor default = %q", got)
	}
}

// Cursor returns well over a hundred models, which is not something a
// user can arrow through. Past a threshold the picker has to filter.
func TestLongModelListsGetFiltering(t *testing.T) {
	cfg := config.Default()
	many := make([]config.ModelChoice, 0, 40)
	for i := 0; i < 40; i++ {
		many = append(many, config.ModelChoice{Label: "M", Model: string(rune('a'+i%26)) + "-model"})
	}
	cfg.Cursor.ModelChoices = many

	var v string
	cursorP, claudeP := "cursor", "claude"
	long := modelField(cfg, &cursorP, "work", &v)
	short := modelField(cfg, &claudeP, "work", &v)
	if long == nil || short == nil {
		t.Fatal("nil field")
	}
	// The built-in Claude list is four entries; the Cursor one here is
	// forty. Only the second should be filterable, and the threshold is
	// what decides — assert on it rather than on huh's internals.
	if len(cfg.Claude.Choices())+1 > modelFilterThreshold {
		t.Errorf("the built-in Claude list (%d) is above the filter threshold (%d); the constant needs raising or the list trimming",
			len(cfg.Claude.Choices()), modelFilterThreshold)
	}
	if len(many)+1 <= modelFilterThreshold {
		t.Errorf("test fixture is too small to exercise filtering")
	}
}

// Switching provider must change the model list. Built from a value
// rather than a bound pointer, the list froze at form-construction
// time: picking Cursor still offered Claude ids, and an explicit pick
// went out as `agent --model claude-sonnet-4-6`.
func TestModelOptionsFollowTheProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.ModelChoices = []config.ModelChoice{{Label: "Opus", Model: "claude-opus-4-8"}}
	cfg.Cursor.ModelChoices = []config.ModelChoice{{Label: "Codex", Model: "gpt-5.3-codex"}}

	claudeOpts := modelOptions(cfg, "claude", "work")
	cursorOpts := modelOptions(cfg, "cursor", "work")

	has := func(opts []huh.Option[string], id string) bool {
		for _, o := range opts {
			if o.Value == id {
				return true
			}
		}
		return false
	}
	if !has(claudeOpts, "claude-opus-4-8") || has(claudeOpts, "gpt-5.3-codex") {
		t.Errorf("claude options are wrong: %+v", claudeOpts)
	}
	if !has(cursorOpts, "gpt-5.3-codex") || has(cursorOpts, "claude-opus-4-8") {
		t.Errorf("cursor options leak Claude ids: %+v", cursorOpts)
	}
	// Both must lead with the no-preference entry.
	for _, opts := range [][]huh.Option[string]{claudeOpts, cursorOpts} {
		if len(opts) == 0 || opts[0].Value != "" {
			t.Errorf("first option is not Default: %+v", opts)
		}
	}
}

// A bounded call, for the same reason CreateChat is bounded: this one
// runs while the form is being drawn.
func TestListModelsIsBounded(t *testing.T) {
	if listModelsTimeout <= 0 {
		t.Error("--list-models has no deadline; a hung CLI would freeze the new-track form")
	}
}

// A sync.Once here pinned the first failure for the life of the
// process: one blip while not yet logged in and the picker showed
// "Default" alone until tracks was restarted, with no way to retry.
// Only successes may be cached.
func TestModelListFailureIsNotCached(t *testing.T) {
	cursorModelCache.mu.Lock()
	cursorModelCache.loaded, cursorModelCache.models = false, nil
	cursorModelCache.mu.Unlock()

	cfg := config.Default()
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "not-installed")
	if _, err := cursorModels(cfg); err == nil {
		t.Fatal("expected a failure from a missing binary")
	}

	// A later call with a working binary must succeed rather than
	// replaying the cached failure.
	good := filepath.Join(t.TempDir(), "agent")
	script := "#!/bin/sh\necho 'Available models'\necho 'auto - Auto (current, default)'\necho 'gpt-5.3-codex - Codex 5.3'\n"
	if err := os.WriteFile(good, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Cursor.Binary = good
	got, err := cursorModels(cfg)
	if err != nil {
		t.Fatalf("retry after a failure should succeed, got: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("parsed %d models on retry, want 2", len(got))
	}

	// And the success IS cached: a later call with a broken binary
	// returns the cached list rather than erroring.
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "gone")
	again, err := cursorModels(cfg)
	if err != nil || len(again) != 2 {
		t.Errorf("success was not cached: got %d models, err=%v", len(again), err)
	}

	cursorModelCache.mu.Lock()
	cursorModelCache.loaded, cursorModelCache.models = false, nil
	cursorModelCache.mu.Unlock()
}

// Exit 0 with nothing parseable is the same trap as an outright
// failure: caching it pins an empty picker for the whole process. The
// CLI can plausibly do this — a login hint, a format change, the
// catalogue on stderr.
func TestEmptyModelListIsNotCached(t *testing.T) {
	cursorModelCache.mu.Lock()
	cursorModelCache.loaded, cursorModelCache.models = false, nil
	cursorModelCache.mu.Unlock()
	t.Cleanup(func() {
		cursorModelCache.mu.Lock()
		cursorModelCache.loaded, cursorModelCache.models = false, nil
		cursorModelCache.mu.Unlock()
	})

	quiet := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(quiet, []byte("#!/bin/sh\necho 'Run agent login to continue'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Cursor.Binary = quiet
	if _, err := cursorModels(cfg); err == nil {
		t.Fatal("an empty catalogue should be an error, not a cached empty list")
	}

	good := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(good, []byte("#!/bin/sh\necho 'auto - Auto'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Cursor.Binary = good
	got, err := cursorModels(cfg)
	if err != nil || len(got) != 1 {
		t.Errorf("retry after an empty result should succeed: %d models, err=%v", len(got), err)
	}
}
