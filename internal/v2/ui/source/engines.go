package source

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
)

// Engines keeps the engines' settings and asks their CLIs.
type Engines interface {
	Load() (settings.Engines, error)
	Save(settings.Engines) error
	// Check finds e's CLI; agents.ErrNotFound when it isn't installed.
	Check(ctx context.Context, e agents.Engine) (agents.Found, error)
	// Models asks the CLI at path for its models.
	Models(ctx context.Context, path string) ([]agents.Model, error)
	// MCP asks the CLI at path for the MCP servers every project gets.
	MCP(ctx context.Context, path string) ([]agents.MCPServer, error)
}
