package agents_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/agents/claude"
	"github.com/bluegardenproject/tracks/internal/v2/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/v2/golden"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// tempDir is where the golden files show a case's temporary folder.
const tempDir = "/tmp/track"

// TestCommandsMatchGolden keeps every start and resume command, prompts
// included, in testdata: a prompt changes only on purpose.
func TestCommandsMatchGolden(t *testing.T) {
	engines := []struct {
		name    string
		program string
		command func(agents.Spec) (agents.Start, error)
	}{
		{"claude", "claude", claude.Command},
		{"cursor", "agent", cursor.Command},
	}
	for _, e := range engines {
		for _, resume := range []bool{false, true} {
			var out strings.Builder
			for _, c := range spawnCases {
				v2, _, _ := c.tracks(t)
				s := c.spec(v2, e.program)
				s.Resume = resume
				got, err := e.command(s)
				if err != nil {
					t.Fatalf("%s %s: %v", e.name, c.name, err)
				}
				text := fmt.Sprintf("=== %s\ndir: %s\n\n%s\n\n", c.name, got.Dir, got.Command)
				if dir := caseDir(v2); dir != "" {
					text = strings.ReplaceAll(text, dir, tempDir)
				}
				if home, err := os.UserHomeDir(); err == nil && got.Dir == home {
					text = strings.Replace(text, "dir: "+home+"\n", "dir: ~\n", 1)
				}
				out.WriteString(text)
			}
			name := e.name
			if resume {
				name += "-resume"
			}
			golden.Check(t, filepath.Join("testdata", name+".golden"), out.String())
		}
	}
}

// caseDir is the temporary folder a case's repos and document are in.
func caseDir(t track.Track) string {
	switch {
	case len(t.Repos) > 0:
		return filepath.Dir(t.Repos[0].Path)
	case t.Document != "":
		return filepath.Dir(t.Document)
	}
	return ""
}
