package cli

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// engines keeps the engines section of settings.yaml for the Engines tab.
type engines struct{ path string }

var _ source.Engines = engines{}

func (e engines) Load() (settings.Engines, error) {
	s, err := settings.Load(e.path)
	return s.Engines, err
}

func (e engines) Save(en settings.Engines) error {
	s, err := settings.Load(e.path)
	if err != nil {
		return err
	}
	s.Engines = en
	return settings.Save(e.path, s)
}

func (engines) Check(ctx context.Context, e agents.Engine) (agents.Found, error) {
	return agents.Check(ctx, e)
}

func (engines) Models(ctx context.Context, path string) ([]agents.Model, error) {
	return agents.ListModels(ctx, path)
}

func (engines) MCP(ctx context.Context, path string) ([]agents.MCPServer, error) {
	return agents.ListMCP(ctx, path)
}
