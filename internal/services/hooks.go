package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// RunHooks runs each command in order via the user's login shell in dir,
// after rendering it against data (so hooks can reference {{.Port "x"}}
// etc.). Output is appended to logPath. The first failing hook aborts and
// returns its error; an empty list is a no-op.
//
// Hooks are where repo-specific wiring lives — e.g. patching a live-app
// manifest URL with the allocated port — so the binary stays generic.
func RunHooks(ctx context.Context, cmds []string, data TemplateData, dir, logPath string) error {
	if len(cmds) == 0 {
		return nil
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open hook log: %w", err)
	}
	defer logf.Close()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	for _, raw := range cmds {
		rendered, err := Render(raw, data)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, shell, "-lc", rendered)
		cmd.Dir = dir
		cmd.Env = os.Environ()
		cmd.Stdout = logf
		cmd.Stderr = logf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q: %w", rendered, err)
		}
	}
	return nil
}
