package newtrack

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/huh"
)

// providerField is the agent-CLI picker, shown before the model one
// because it decides which models exist.
//
// Callers reach it through providerFields, which omits it entirely
// when only one provider is installed.
func providerField(cfg config.Config, v *string) *huh.Select[string] {
	options := make([]huh.Option[string], 0, len(state.Providers()))
	for _, p := range providerChoices(cfg) {
		options = append(options, huh.NewOption(p.Label(), string(p)))
	}
	return huh.NewSelect[string]().
		Title("Provider").
		Description("Which agent CLI runs this track. Claude Code reads CLAUDE.md and can hand its diff to the tracks review subagent; Cursor Agent reaches models Claude Code can't (GPT, Gemini, Grok, Composer) and reviews its own work instead.").
		Options(options...).
		Value(v)
}

// providerChoices lists the providers actually available on this
// machine. A provider whose binary is missing is left out rather than
// offered and then failing at spawn.
//
// Claude is always listed: it is the default, and a tracks install
// without it is already broken in ways this picker cannot help with.
func providerChoices(cfg config.Config) []state.Provider {
	out := []state.Provider{state.ProviderClaude}
	binary := strings.TrimSpace(cfg.Cursor.Binary)
	if binary == "" {
		binary = "agent"
	}
	if _, err := exec.LookPath(binary); err == nil {
		out = append(out, state.ProviderCursor)
	}
	return out
}

// defaultProvider is the provider the form starts on: the configured
// one, unless it isn't available here.
func defaultProvider(cfg config.Config) string {
	want := state.Provider(strings.TrimSpace(cfg.Provider)).Resolved()
	for _, p := range providerChoices(cfg) {
		if p == want {
			return string(p)
		}
	}
	return string(state.ProviderClaude)
}

// listModelsTimeout bounds the model-list call. Shorter than
// CreateChat's: a user is watching a form redraw, not a track being
// provisioned, and the fallback (an unfiltered "Default") is harmless.
const listModelsTimeout = 10 * time.Second

// cursorModelCache holds the model list for the life of one TUI
// process. `agent --list-models` is a network round trip and the flow
// rebuilds its form several times (the discard-confirm loop), which
// would otherwise mean one call per redraw.
//
// Only successes are cached. A sync.Once here would pin the first
// failure too — not logged in yet, a blip, a slow reply that hit the
// timeout — and the picker would then show "Default" alone until the
// user quit and restarted tracks, with no way to retry.
var cursorModelCache struct {
	mu     sync.Mutex
	models []config.ModelChoice
	loaded bool
}

// cursorModels asks the CLI what this account can run.
//
// There is no useful built-in list to fall back on the way there is
// for Claude: Cursor's catalogue is account-specific and its ids are
// its own ("gpt-5.3-codex", "composer-2.5"). When the call fails the
// picker degrades to just "Default", which passes no --model and lets
// the binary choose — never a wrong model, only an unchosen one.
func cursorModels(cfg config.Config) ([]config.ModelChoice, error) {
	cursorModelCache.mu.Lock()
	defer cursorModelCache.mu.Unlock()
	if cursorModelCache.loaded {
		return cursorModelCache.models, nil
	}

	binary := strings.TrimSpace(cfg.Cursor.Binary)
	if binary == "" {
		binary = "agent"
	}
	{
		// Bounded for the same reason CreateChat is: this CLI can hang,
		// and this call happens while building the new-track form. An
		// unbounded one freezes the TUI with no error and no way out.
		ctx, cancel := context.WithTimeout(context.Background(), listModelsTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "--list-models")
		cmd.Stdin = nil
		cmd.WaitDelay = 5 * time.Second
		out, err := cmd.Output()
		if err != nil {
			// Deliberately not cached: the next redraw retries.
			return nil, fmt.Errorf("%s --list-models: %w", binary, err)
		}
		models := parseModelList(string(out))
		if len(models) == 0 {
			// Exit 0 with nothing parseable: a login hint, a changed
			// format, the catalogue on stderr. Caching that pins an
			// empty picker for the rest of the process, which is the
			// same trap as caching an outright failure.
			return nil, fmt.Errorf("%s --list-models printed no models", binary)
		}
		cursorModelCache.models = models
		cursorModelCache.loaded = true
	}
	return cursorModelCache.models, nil
}

// parseModelList reads the `id - Label` lines `agent --list-models`
// prints, ignoring its header and any warning noise around them.
func parseModelList(out string) []config.ModelChoice {
	var models []config.ModelChoice
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		id, label, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		id, label = strings.TrimSpace(id), strings.TrimSpace(label)
		// An id is a single token; anything with a space in it is prose
		// that happened to contain " - ".
		if id == "" || label == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		models = append(models, config.ModelChoice{Label: label, Model: id})
	}
	return models
}

// modelChoicesFor returns the picker entries for a provider.
//
// Claude's come from config, which has a sensible built-in list.
// Cursor's come from the binary, because only it knows what the
// account can reach.
func modelChoicesFor(cfg config.Config, provider string) []config.ModelChoice {
	if state.Provider(provider).Resolved() != state.ProviderCursor {
		return cfg.Claude.Choices()
	}
	if len(cfg.Cursor.ModelChoices) > 0 {
		return cfg.Cursor.ModelChoices
	}
	models, err := cursorModels(cfg)
	if err != nil {
		// Degrade to "Default" only. Guessing a list would offer models
		// the account may not have.
		return nil
	}
	return models
}

// defaultModelFor names the configured default for a provider, for the
// picker's first option.
func defaultModelFor(cfg config.Config, provider, kind string) string {
	if state.Provider(provider).Resolved() == state.ProviderCursor {
		return cfg.Cursor.ModelFor(kind)
	}
	return cfg.Claude.ModelFor(kind)
}

// providerFields returns the provider picker, or nothing when there is
// no choice to make.
//
// A select with a single option is a keypress that teaches the user
// nothing, on a form they fill in every time they start a track. A
// Claude-only install should look exactly as it did before Cursor
// support existed.
func providerFields(cfg config.Config, v *string) []huh.Field {
	if len(providerChoices(cfg)) < 2 {
		return nil
	}
	return []huh.Field{providerField(cfg, v)}
}
