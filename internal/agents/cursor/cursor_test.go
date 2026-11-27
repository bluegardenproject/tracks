package cursor

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestHooksGoInAsAPlugin(t *testing.T) {
	spec := agents.Spec{Track: track.Track{ID: "t1", Kind: track.Ask, Session: "c1"}, Program: "agent", Hooks: "/data/hooks/t1/tracks", Token: "tok"}
	start, err := Command(spec)
	if err != nil || !strings.Contains(start.Command, "--plugin-dir") || !strings.Contains(start.Command, "/data/hooks/t1/tracks") {
		t.Errorf("Command = %q, %v", start.Command, err)
	}
	if !strings.Contains(start.Command, "TRACKS_AGENT_TOKEN='tok'") || strings.Contains(start.Command, "--approve-mcps") {
		t.Errorf("the token or the MCP approval: %q", start.Command)
	}
	spec.Hooks = ""
	if start, _ := Command(spec); strings.Contains(start.Command, "--plugin-dir") {
		t.Errorf("without hooks: %q", start.Command)
	}
}
