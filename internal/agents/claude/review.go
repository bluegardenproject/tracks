package claude

import "github.com/bluegardenproject/tracks/internal/review"

// ReviewCommand starts Claude Code as a headless reviewer on model, ""
// for Claude's own default. It can read files and nothing else:
//
//   - --tools leaves only Read, Grep and Glob. No shell: allowing even
//     `git diff` lets a write through on a setup that runs sandboxed
//     commands without asking, which a live check showed.
//   - --restricted ignores the user's and the project's settings,
//     whose permission rules would add to the reviewer's.
//   - --strict-mcp-config with no config drops MCP servers, which can
//     post to chats or change tickets.
//
// So it can't change the worktree, push, or start another reviewer.
func ReviewCommand(binary, model string) review.Command {
	if binary == "" {
		binary = "claude"
	}
	args := []string{"-p", "--restricted", "--tools", "Read,Grep,Glob", "--strict-mcp-config",
		"--permission-mode", "default"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return review.Command{
		Binary: binary, Args: args,
		Tools: "You can read files and nothing else, so skip any instruction " +
			"above that asks you to run git or any other command: the diff is " +
			"below, and you can read files in the workspace for context.",
	}
}
