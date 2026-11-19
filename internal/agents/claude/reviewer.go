package claude

import "github.com/bluegardenproject/tracks/internal/agents"

// ReviewerLine starts Claude Code as a track's reviewer, in plan mode
// with Read, Grep and Glob its only tools and no MCP servers: it reads,
// and runs and writes nothing. A shell would let a write through where
// sandboxed commands run without asking, and MCP servers can post to
// chats or change tickets. Editing tools and ExitPlanMode are denied as
// well, so it never waits on a plan nobody approves in a hidden pane. hooks is the reviewer's settings file, which reports its
// replies. The brief comes in as part of its system prompt, from a file
// Claude Code reads itself: a folder of the brief's given with --add-dir
// would make it ask, in the hidden pane, whether to trust it.
func ReviewerLine(program, model, hooks, brief, prompt string) agents.CommandLine {
	line := agents.NewLine(program)
	line.Arg(prompt)
	line.Set("--permission-mode", "plan")
	line.Set("--tools", "Read,Grep,Glob")
	line.Set("--disallowedTools", "Edit,Write,NotebookEdit,ExitPlanMode")
	line.Flag("--strict-mcp-config")
	line.Set("--append-system-prompt-file", brief)
	line.SetIf("--model", model)
	line.SetIf("--settings", hooks)
	return line.Build()
}
