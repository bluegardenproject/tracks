package tracks

import (
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/v2/hooks"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// installHooks writes t's hooks and returns what its engine is started
// with; "" without a HooksDir. They report through the tracks command
// in BinDir, which runs the daemon's build.
func (s *Service) installHooks(t track.Track) (string, error) {
	if s.HooksDir == "" {
		return "", nil
	}
	command := hooks.Command(filepath.Join(s.BinDir, "tracks"), t.Engine, t.ID)
	return hooks.Install(filepath.Join(s.HooksDir, t.ID), t.Engine, command)
}

// removeHooks removes track id's hooks.
func (s *Service) removeHooks(id string) {
	if s.HooksDir != "" {
		_ = os.RemoveAll(filepath.Join(s.HooksDir, id))
	}
}
