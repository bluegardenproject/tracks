package cli

import (
	"os"

	"github.com/bluegardenproject/tracks/internal/v2/demo"
	"github.com/spf13/cobra"
)

// newDemoCmd holds the playground's fake processes.
func newDemoCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "demo", Hidden: true}

	var scenario, track string
	agent := &cobra.Command{
		Use:  "agent",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return demo.RunAgent(c.Context(), c.OutOrStdout(), os.Stdin, demo.Scenario(scenario), track)
		},
	}
	agent.Flags().StringVar(&scenario, "scenario", string(demo.Implementing), "what the agent acts out")
	agent.Flags().StringVar(&track, "track", "", "the track's name")

	var name string
	var port int
	devserver := &cobra.Command{
		Use:  "devserver",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return demo.RunDevServer(c.Context(), c.OutOrStdout(), name, port)
		},
	}
	devserver.Flags().StringVar(&name, "name", "web", "the server's name")
	devserver.Flags().IntVar(&port, "port", 3000, "the port it pretends to listen on")

	cmd.AddCommand(agent, devserver)
	return cmd
}
