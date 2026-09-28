package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/agents/claude"
	"github.com/bluegardenproject/tracks/internal/v2/agents/cursor"
)

// Engine starts one agent CLI on tracks.
type Engine interface {
	// Session makes the ID the track's conversation keeps for life.
	Session(ctx context.Context, program string) (string, error)
	Command(s agents.Spec) (agents.Start, error)
}

var engines = map[string]Engine{
	agents.Claude.ID: claudeEngine{},
	agents.Cursor.ID: cursorEngine{},
}

type claudeEngine struct{}

func (claudeEngine) Session(context.Context, string) (string, error) { return claude.NewSession(), nil }
func (claudeEngine) Command(s agents.Spec) (agents.Start, error)     { return claude.Command(s) }

type cursorEngine struct{}

func (cursorEngine) Session(ctx context.Context, program string) (string, error) {
	return cursor.CreateChat(ctx, program)
}
func (cursorEngine) Command(s agents.Spec) (agents.Start, error) { return cursor.Command(s) }

func (s *Service) engine(id string) (Engine, bool) {
	all := s.Engines
	if all == nil {
		all = engines
	}
	e, ok := all[id]
	return e, ok
}
