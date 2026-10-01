package cli

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/ui/source"
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

// trackTypes keeps the tracks section of settings.yaml for Settings.
type trackTypes struct{ path string }

var _ source.TrackTypes = trackTypes{}

func (t trackTypes) Load() (settings.Tracks, error) {
	s, err := settings.Load(t.path)
	return s.Tracks, err
}

func (t trackTypes) Save(tracks settings.Tracks) error {
	s, err := settings.Load(t.path)
	if err != nil {
		return err
	}
	s.Tracks = tracks
	return settings.Save(t.path, s)
}

type history struct{ path string }

var _ source.History = history{}

func (h history) Load() (settings.History, error) {
	s, err := settings.Load(h.path)
	return s.History, err
}

func (h history) Save(history settings.History) error {
	s, err := settings.Load(h.path)
	if err != nil {
		return err
	}
	s.History = history
	return settings.Save(h.path, s)
}

type notifications struct{ path string }

var _ source.Notifications = notifications{}

func (n notifications) Load() (settings.Notifications, error) {
	s, err := settings.Load(n.path)
	return s.Notifications, err
}

func (n notifications) Save(notify settings.Notifications) error {
	s, err := settings.Load(n.path)
	if err != nil {
		return err
	}
	s.Notifications = notify
	return settings.Save(n.path, s)
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
