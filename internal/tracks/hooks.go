package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// agentSetup is what a track's agent starts with besides its prompt:
// its hooks, Tracks' MCP server and the token it knows the agent by.
type agentSetup struct{ Hooks, MCPConfig, Token string }

// spec puts a in s.
func (a agentSetup) spec(s agents.Spec) agents.Spec {
	s.Hooks, s.MCPConfig, s.Token = a.Hooks, a.MCPConfig, a.Token
	return s
}

// installHooks writes t's hooks and Tracks' MCP server, and gives its
// agent a new token; nothing without a HooksDir. They run the tracks
// command in BinDir, which runs the daemon's build. The token counts
// once saveToken has kept it.
func (s *Service) installHooks(t track.Track) (agentSetup, error) {
	if s.HooksDir == "" {
		return agentSetup{}, nil
	}
	program := filepath.Join(s.BinDir, "tracks")
	command := hooks.Command(program, t.Engine, t.ID)
	h, mcp, err := hooks.InstallAgent(filepath.Join(s.HooksDir, t.ID), t.Engine, command, s.Socket, program)
	if err != nil {
		return agentSetup{}, err
	}
	return agentSetup{Hooks: h, MCPConfig: mcp, Token: agents.NewToken()}, nil
}

// saveToken keeps a's token as the one of track id's agent, in place of
// the one its last session had.
func (s *Service) saveToken(ctx context.Context, id string, a agentSetup) error {
	if a.Token == "" {
		return nil
	}
	return s.Store.SetAgentToken(ctx, id, agents.HashToken(a.Token))
}

// AgentTrack is the track whose agent holds token: what tracks mcp
// shows the daemon.
func (s *Service) AgentTrack(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", Problem("No agent token: tracks mcp runs in an agent Tracks started.")
	}
	id, err := s.Store.AgentToken(ctx, agents.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return "", Problem("Tracks doesn't know this agent's token: its track ended or its agent restarted.")
	}
	return id, err
}

// removeHooks removes track id's hooks.
func (s *Service) removeHooks(id string) {
	if s.HooksDir != "" {
		_ = os.RemoveAll(filepath.Join(s.HooksDir, id))
	}
}
