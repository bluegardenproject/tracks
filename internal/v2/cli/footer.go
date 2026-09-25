package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/footer"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/sysinfo"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/spf13/cobra"
)

// wanLookup returns the public address as plain text.
const wanLookup = "https://api.ipify.org"

// newFooterCmd holds the commands the footer's rows run.
func newFooterCmd(profile profileFunc) *cobra.Command {
	cmd := &cobra.Command{Use: "footer", Hidden: true}

	var light bool
	system := &cobra.Command{
		Use:  "system",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			paths, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			ctx := c.Context()
			s := sysinfo.Snapshot{LAN: sysinfo.LAN()}
			s.WAN = sysinfo.WAN{
				URL:       wanLookup,
				CacheFile: filepath.Join(paths.DataDir, "wan.json"),
				MaxAge:    10 * time.Minute,
				Timeout:   3 * time.Second,
			}.Address(ctx)
			if cpu, err := sysinfo.CPU(ctx, 500*time.Millisecond); err == nil {
				s.CPU, s.CPUKnown = cpu, true
			}
			s.MemUsed, s.MemTotal, _ = sysinfo.Memory(ctx)
			t, _ := theme.Load(themePath(paths))
			_, err = fmt.Fprintln(c.OutOrStdout(), footer.System(s, t, !light))
			return err
		},
	}
	system.Flags().BoolVar(&light, "light", false, "colours for a light background")

	cmd.AddCommand(system)
	return cmd
}
