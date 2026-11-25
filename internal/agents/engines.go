// Package agents knows the engines, the agent CLIs tracks run on:
// their programs, whether one is installed, and the models it offers.
package agents

// Engine is an agent CLI Tracks can run.
type Engine struct {
	ID      string // the key in settings.yaml
	Name    string
	Program string // found on PATH
	Install string // a command that installs it
	// InstallDocs, for an engine without one command that installs it
	// everywhere, is the page that says how.
	InstallDocs string
	// Models is the built-in model list, for engines that can't list
	// their own; ListsModels engines ask the CLI instead.
	Models      []Model
	ListsModels bool
	// MCPNote says which MCP servers `mcp list` leaves out.
	MCPNote string
	// Preview engines are found and added on the Engines tab, but
	// Tracks can't run tracks on them yet.
	Preview bool
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
	Copilot = Engine{
		ID: "copilot", Name: "GitHub Copilot", Program: "copilot",
		InstallDocs: "https://docs.github.com/copilot/how-tos/copilot-cli",
		Preview:     true,
	}
)

// All are the engines, in the order they're shown.
var All = []Engine{Claude, Cursor, Copilot}

// Runners are the engines tracks run on: All but the previews.
func Runners() []Engine {
	var out []Engine
	for _, e := range All {
		if !e.Preview {
			out = append(out, e)
		}
	}
	return out
}

// ByID returns the engine id.
func ByID(id string) (Engine, bool) {
	for _, e := range All {
		if e.ID == id {
			return e, true
		}
	}
	return Engine{}, false
}
