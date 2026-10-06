package cli

import (
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

// newReportExitCmd is what a setup or dev-server pane runs when its
// command ends, so a failure shows on Station.
func newReportExitCmd() *cobra.Command {
	var p rpc.ReportExitParams
	cmd := &cobra.Command{
		Use:    "report-exit",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("report-exit")
			if err != nil {
				return err
			}
			p.ID = id
			return client.ReportExit(c.Context(), p)
		},
	}
	cmd.Flags().StringVar(&p.Kind, "kind", "", "setup or server")
	cmd.Flags().StringVar(&p.Subject, "subject", "", "the repo, or repo/server")
	cmd.Flags().IntVar(&p.Code, "code", 0, "the exit code")
	return cmd
}
