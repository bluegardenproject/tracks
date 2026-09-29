package claude

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestHooksGoInWithSettings(t *testing.T) {
	spec := agents.Spec{Track: track.Track{ID: "t1", Kind: track.Ask, Session: "s1"}, Program: "claude", Hooks: "/data/hooks/t1/claude-settings.json"}
	start, err := Command(spec)
	if err != nil || !strings.Contains(start.Command, "--settings") || !strings.Contains(start.Command, "/data/hooks/t1/claude-settings.json") {
		t.Errorf("Command = %q, %v", start.Command, err)
	}
	spec.Hooks = ""
	if start, _ := Command(spec); strings.Contains(start.Command, "--settings") {
		t.Errorf("without hooks: %q", start.Command)
	}
}
