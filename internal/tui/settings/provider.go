package settings

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/huh"
)

// providerMenuLabel is the Provider row on the settings menu, naming
// the current default so it is visible without opening the section.
func providerMenuLabel(cfg config.Config) string {
	return fmt.Sprintf("Provider  (default: %s)", defaultProviderLabel(cfg))
}

// defaultProviderLabel renders the configured default provider, falling
// back to Claude's label the same way an unset field resolves.
func defaultProviderLabel(cfg config.Config) string {
	return state.Provider(strings.TrimSpace(cfg.Provider)).Resolved().Label()
}

// editProvider picks the agent CLI new tracks default to. It is only a
// default: the creation form pre-selects it and the user can override
// it per track, which is why this form says so rather than reading as
// a global switch.
func editProvider(cfg *config.Config) error {
	current := string(state.Provider(strings.TrimSpace(cfg.Provider)).Resolved())

	options := make([]huh.Option[string], 0, len(state.Providers()))
	for _, p := range state.Providers() {
		options = append(options, huh.NewOption(p.Label(), string(p)))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default provider").
				Description("Which agent CLI new tracks run. The creation form pre-selects it and you can override it per track.\n\nClaude Code reads CLAUDE.md and hands its diff to the tracks review subagent. Cursor Agent reaches models Claude Code can't (GPT, Gemini, Grok, Composer), takes its instructions from ~/.cursor/rules, and reviews its own work instead.\n\nCursor is only offered at creation when its binary is on PATH.").
				Options(options...).
				Value(&current),
		),
	)
	if err := runSettingsForm(form); err != nil {
		return err
	}
	cfg.Provider = current
	return nil
}
