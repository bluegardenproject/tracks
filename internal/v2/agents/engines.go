// Package agents knows the engines, the agent CLIs tracks run on:
// their programs, whether one is installed, and the models it offers.
package agents

// Engine is an agent CLI Tracks can run.
type Engine struct {
	ID      string // the key in settings.yaml
	Name    string
	Program string // found on PATH
	Install string // a command that installs it
	// Models is the built-in model list, for engines that can't list
	// their own; ListsModels engines ask the CLI instead.
	Models      []Model
	ListsModels bool
	// MCPNote says which MCP servers `mcp list` leaves out.
	MCPNote string
}

// Model is a value for the CLI's --model, and what to call it.
type Model struct {
	ID, Label string
}

var (
	Claude = Engine{
		ID: "claude", Name: "Claude Code", Program: "claude",
		Install: "curl -fsSL https://claude.ai/install.sh | bash",
		// Aliases follow each family's newest release; users pin
		// versions by adding their ids.
		Models: []Model{
			{"opus", "Opus (latest)"},
			{"sonnet", "Sonnet (latest)"},
			{"haiku", "Haiku (latest)"},
			{"fable", "Fable (latest)"},
		},
	}
	Cursor = Engine{
		ID: "cursor", Name: "Cursor", Program: "agent",
		Install:     "curl https://cursor.com/install -fsS | bash",
		ListsModels: true,
		MCPNote:     "Servers from Cursor plugins, such as Figma, aren't listed here: the Cursor CLI only shows them in /mcp list inside a session.",
	}
)

// All are the engines, in the order they're shown.
var All = []Engine{Claude, Cursor}

// ByID returns the engine id.
func ByID(id string) (Engine, bool) {
	for _, e := range All {
		if e.ID == id {
			return e, true
		}
	}
	return Engine{}, false
}
