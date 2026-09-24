package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

// Seed creates a directory under dataDir and a window in session for
// every fake track. command runs this Tracks with the demo profile; the
// fake agents and dev servers are its hidden `demo` subcommands.
func Seed(t trackwin.Tmux, session, dataDir, command string) error {
	for _, tr := range Tracks() {
		dir := filepath.Join(dataDir, "worktrees", tr.Repo, tr.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		readme := fmt.Sprintf("# %s\n\nA fake %s track in the %s repo, for the Tracks v2 playground.\n", tr.Name, tr.Kind, tr.Repo)
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
			return err
		}

		spec := trackwin.Spec{
			Name: tr.Name,
			Dir:  dir,
			Agent: trackwin.Process{
				Title:   "agent",
				Command: command + " demo agent --scenario " + string(tr.Scenario) + " --track " + tr.Name,
			},
			Terminals: 1,
		}
		if ds := tr.DevServer; ds != nil {
			port := strconv.Itoa(ds.Port)
			spec.DevServers = []trackwin.Process{{
				Title:   ds.Name + ":" + port,
				Command: command + " demo devserver --name " + ds.Name + " --port " + port,
			}}
		}
		if _, err := trackwin.Open(t, session, spec); err != nil {
			return fmt.Errorf("open %s: %w", tr.Name, err)
		}
	}
	return nil
}
