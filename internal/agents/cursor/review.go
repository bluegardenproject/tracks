package cursor

import "github.com/bluegardenproject/tracks/internal/review"

// ReviewCommand starts Cursor's agent as a reviewer in workspace, on
// model, "" for Cursor's own default.
//
// **No permission flags are passed.** Not --force, not --trust, not
// --sandbox. Whatever the user or their organisation has configured
// applies unchanged, and on a default `allowlist` setup that means the
// reviewer cannot execute anything at all: it reads files and reports.
// That is deliberate on three counts — a reviewer needs no write
// access, overriding someone's permission policy is not ours to do,
// and a process that cannot execute cannot spawn another reviewer.
func ReviewCommand(binary, workspace, model string) review.Command {
	if binary == "" {
		binary = "agent"
	}
	args := []string{"-p", "--workspace", workspace}
	if model != "" {
		args = append(args, "--model", model)
	}
	return review.Command{
		Binary: binary, Args: args,
		Tools: "You are running without permission to execute commands, so " +
			"skip any instruction above that asks you to run git or any " +
			"other tool: the diff is below and you can read files in the " +
			"workspace for context.",
	}
}
