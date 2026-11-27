package claude

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
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

func TestEffortGoesInOnStartAndResume(t *testing.T) {
	spec := agents.Spec{Track: track.Track{ID: "t1", Kind: track.Ask, Session: "s1", Model: "opus", Effort: "high"}, Program: "claude"}
	for _, resume := range []bool{false, true} {
		spec.Resume = resume
		if start, err := Command(spec); err != nil || !strings.Contains(start.Command, `--effort '\''high`) {
			t.Errorf("resume %v: Command = %q, %v", resume, start.Command, err)
		}
	}
	spec.Track.Effort = ""
	if start, _ := Command(spec); strings.Contains(start.Command, "--effort") {
		t.Errorf("without an effort: %q", start.Command)
	}
}
