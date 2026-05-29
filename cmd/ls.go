package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "ls",
		Short: "list all known tracks",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			tracks, err := cl.Ls()
			if err != nil {
				return fmt.Errorf("ls: %w", err)
			}
			if len(tracks) == 0 {
				fmt.Println("no tracks yet — run `tracks new` to create one")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tBRANCH\tSTATUS\tREPOS\tUPDATED")
			for _, t := range tracks {
				repos := ""
				for i, r := range t.Repos {
					if i > 0 {
						repos += ","
					}
					repos += r.Name
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					t.ID, t.Branch, t.Status, repos, t.UpdatedAt.Format("2006-01-02 15:04:05"))
			}
			return tw.Flush()
		},
	})
}
