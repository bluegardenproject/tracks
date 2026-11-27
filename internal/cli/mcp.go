package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/mcp"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/spf13/cobra"
)

func newMCPCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "serve Tracks' tools to the agent of a track over MCP, on stdio",
		Long: "An MCP server on stdio, which the agent's engine starts: the relay, the dev servers and the panes " +
			"of the agent's track, as tools. The agent is known by $" + agents.TokenEnv + ", which Tracks gives " +
			"each agent it starts; without it, no tools.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			socket, err := mcpSocket(os.Getenv("TRACKS_SOCKET_DIR"))
			if err != nil {
				return err
			}
			env := mcpEnv{Token: os.Getenv(agents.TokenEnv), Track: os.Getenv("TRACKS_ID"),
				Reviewer: os.Getenv(review.EnvMarker) != ""}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			server := mcp.Server{Name: "tracks", Version: version, Tools: mcpTools(env, socket, paths.TmuxSocket)}
			return server.Serve(c.Context(), c.InOrStdin(), c.OutOrStdout())
		},
	}
}

// mcpSocket is the daemon's socket: in dir, the pane's
// TRACKS_SOCKET_DIR, or where Tracks keeps it.
func mcpSocket(dir string) (string, error) {
	if dir != "" {
		return platform.Socket(dir), nil
	}
	paths, err := platform.Resolve()
	return paths.Socket, err
}

// mcpEnv is what tracks mcp knows of its agent: its token, its track
// as the pane says, and whether it's a reviewer.
type mcpEnv struct {
	Token, Track string
	Reviewer     bool
}

// mcpTools are the tools for env's agent, which reach the daemon on
// socket and the track's window on tmuxSocket: none without a token,
// nor for a reviewer, which has no relay of its own to write to.
func mcpTools(env mcpEnv, socket, tmuxSocket string) []mcp.Tool {
	if env.Token == "" || env.Reviewer {
		return nil
	}
	// The daemon answers for the token's track, whatever id a call names.
	client := rpc.Client{Socket: socket, Token: env.Token}
	id := env.Track
	noArgs := map[string]any{"type": "object", "properties": map[string]any{}}
	messages := func(msgs []relay.Message, err error) (string, error) {
		if err != nil {
			return "", err
		}
		return relay.FormatAll(msgs), nil
	}
	return []mcp.Tool{
		{
			Name: "relay_send",
			Description: "Send a train or side track a message: to takes its handle, or a prefix only one of them " +
				"has, such as reviewer. In a side track, to track sends to its main track, and another side track's " +
				"handle asks it a question or answers one, at most " + strconv.Itoa(relay.PairLimit) +
				" messages between the two. kind is question, answer, report or event. outcome gives the message a " +
				"short status: pass, blocked, done or needs-input. refs say what it's about: a PR's URL, #123 for a " +
				"PR of the track's repo, or a file's path in the repo; at most " + strconv.Itoa(relay.MaxRefs) + ".",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to":   map[string]any{"type": "string", "description": "a handle, or a prefix only one has; track for a side track's main track"},
					"kind": map[string]any{"type": "string", "enum": []string{"question", "answer", "report", "event"}},
					"body": map[string]any{"type": "string", "description": "the message"},
					"outcome": map[string]any{"type": "string", "enum": relay.Outcomes,
						"description": "pass, blocked, done or needs-input"},
					"refs": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
						"maxItems": relay.MaxRefs, "description": "a PR's URL, #123, or a file's path in the repo"},
				},
				"required": []string{"to", "kind", "body"},
			},
			Call: func(ctx context.Context, args json.RawMessage) (string, error) {
				var a struct {
					To, Kind, Body string
					Outcome        relay.Outcome
					Refs           []string
				}
				if err := json.Unmarshal(args, &a); err != nil {
					return "", err
				}
				m, err := client.RelaySend(ctx, id, a.To, relay.Kind(a.Kind), a.Body,
					relay.Fields{Outcome: a.Outcome, Refs: a.Refs})
				if err != nil {
					return "", err
				}
				return sent(m), nil
			},
		},
		{
			Name:        "relay_read",
			Description: "The messages this track hasn't read on its relay, now marked read.",
			Schema:      noArgs,
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				return messages(client.RelayRead(ctx, id))
			},
		},
		{
			Name:        "relay_wait",
			Description: "Wait for messages to this track, then return them as relay_read does.",
			Schema:      noArgs,
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				return messages(client.RelayWait(ctx, id))
			},
		},
		{
			Name:        "relay_who",
			Description: "Who's on this track's relay: the track, its trains and its side tracks.",
			Schema:      noArgs,
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				members, err := client.RelayWho(ctx, id)
				if err != nil {
					return "", err
				}
				var out strings.Builder
				err = printMembers(&out, members)
				return out.String(), err
			},
		},
		{
			Name:        "proxy_status",
			Description: "This track's dev servers, with their state and address, and what else listens in its panes.",
			Schema:      noArgs,
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				servers, err := client.Servers(ctx, id)
				if err != nil {
					return "", err
				}
				var out strings.Builder
				err = listServers(&out, servers)
				return out.String(), err
			},
		},
		{
			Name: "panes_list",
			Description: "The panes beside this track's agent: terminals, dev servers, setup, each with the number " +
				"tracks close takes, what runs in it and the ports it listens on.",
			Schema: noArgs,
			Call: func(ctx context.Context, _ json.RawMessage) (string, error) {
				track, err := client.Agent(ctx)
				if err != nil {
					return "", err
				}
				t := tmux.New(tmuxSocket)
				w, err := trackWindow(t, sessionName, track)
				if err != nil {
					return "", fmt.Errorf("list the panes: %w", err)
				}
				var out strings.Builder
				err = listPanes(ctx, &out, t, w)
				return out.String(), err
			},
		},
	}
}
