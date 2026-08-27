package settings

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// An unset provider is the common case — every config written before
// this existed — and must read as Claude rather than blank.
func TestDefaultProviderLabelHandlesAnUnsetField(t *testing.T) {
	if got := defaultProviderLabel(config.Config{}); got != "Claude Code" {
		t.Errorf("unset provider labelled %q, want Claude Code", got)
	}
	if got := defaultProviderLabel(config.Config{Provider: "  "}); got != "Claude Code" {
		t.Errorf("whitespace provider labelled %q, want Claude Code", got)
	}
}

func TestProviderMenuLabelNamesTheCurrentDefault(t *testing.T) {
	got := providerMenuLabel(config.Config{Provider: "cursor"})
	if !strings.Contains(got, "Cursor Agent") {
		t.Errorf("menu label %q does not name the configured provider", got)
	}
	if strings.Contains(got, "cursor") && !strings.Contains(got, "Cursor Agent") {
		t.Errorf("menu label %q shows the raw id rather than the display name", got)
	}
}

// Every provider the form can select has to survive config.Validate,
// or the settings UI can write a config the daemon then rejects.
func TestEveryOfferedProviderValidates(t *testing.T) {
	for _, p := range state.Providers() {
		cfg := config.Default()
		cfg.Repos = []config.Repo{{Name: "demo", Path: "/tmp/demo", Base: "main"}}
		cfg.Provider = string(p)
		if err := cfg.Validate(); err != nil {
			t.Errorf("provider %q is offered by the form but rejected by Validate: %v", p, err)
		}
	}
}
