package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/hooks"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/spf13/cobra"
)

// hookWait is how long a hook waits for the daemon.
const hookWait = time.Second

// newHookCmd is what the agents' hooks run. It never gets in the
// agent's way: it always succeeds, answers as if there were no hook,
// and logs what went wrong.
func newHookCmd() *cobra.Command {
	var engine, id string
	cmd := &cobra.Command{
		Use:    "hook",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return nil
			}
			client := rpc.Client{Socket: paths.Socket}
			logf := func(format string, args ...any) {
				_ = appendLog(filepath.Join(paths.DataDir, "logs", "hook.log"), fmt.Sprintf(format, args...))
			}
			runHook(c.Context(), os.Stdin, c.OutOrStdout(), engine, id, client.Report, logf)
			return nil
		},
	}
	cmd.Flags().StringVar(&engine, "engine", "", "the agent CLI: claude or cursor")
	cmd.Flags().StringVar(&id, "track", "", "the track's ID")
	return cmd
}

// runHook reads a hook's input from in, reports what it means for
// track id, and prints the engine's reply to out.
func runHook(ctx context.Context, in io.Reader, out io.Writer, engine, id string,
	report func(ctx context.Context, id, event string) error, logf func(string, ...any)) {
	p, err := hooks.Read(in)
	defer func() { fmt.Fprint(out, hooks.Reply(engine, p.Event)) }()
	if err != nil {
		logf("%s hook for %s: reading its input: %v", engine, id, err)
		return
	}
	e, ok := hooks.Event(engine, p)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, hookWait)
	defer cancel()
	if err := report(ctx, id, string(e)); err != nil {
		logf("%s %s for %s: %v", engine, p.Event, id, err)
	}
}

func appendLog(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	log.New(f, "", log.LstdFlags).Println(line)
	return nil
}
