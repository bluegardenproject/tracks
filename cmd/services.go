package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

// resolveTrackID returns the track ID from the --track flag (if set),
// falling back to $TRACKS_ID (set automatically when inside a track window).
// errContext is shown in the error message when neither is available.
func resolveTrackID(flagVal, errContext string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if id := os.Getenv("TRACKS_ID"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("%s: --track <id> required (or run from inside a track window where $TRACKS_ID is set)", errContext)
}

func init() {
	// tracks services [--track <id>]
	servicesCmd := &cobra.Command{
		Use:   "services",
		Short: "list dev-server services and their status for a track",
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks services")
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			result, err := cl.Services(id)
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			if len(result.Services) == 0 && len(result.Ports) == 0 {
				fmt.Println("no services configured for this track")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SERVICE\tSTATUS\tPORT\tLOG")
			// Show all configured ports even if not started yet.
			shown := make(map[string]bool)
			for _, ss := range result.Services {
				shown[ss.Name] = true
				port := result.Ports[ss.Name]
				portStr := ""
				if port > 0 {
					portStr = fmt.Sprintf("%d", port)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ss.Name, ss.Status, portStr, ss.LogPath)
			}
			// Ports allocated but not yet started.
			for name, port := range result.Ports {
				if !shown[name] {
					fmt.Fprintf(tw, "%s\t%s\t%d\t\n", name, "not started", port)
				}
			}
			return tw.Flush()
		},
	}
	servicesCmd.Flags().String("track", "", "track ID (defaults to $TRACKS_ID)")
	register(servicesCmd)

	// tracks up <service> [--track <id>]
	upCmd := &cobra.Command{
		Use:   "up <service>",
		Short: "start a dev-server service for a track (and its dependencies)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks up")
			if err != nil {
				return err
			}
			svcName := args[0]
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			result, err := cl.ServiceUpWithProgress(id, svcName, func(msg string) {
				fmt.Println(msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Printf("%s is up — http://localhost:%d\n", svcName, result.Port)
			fmt.Printf("log: %s\n", result.LogPath)
			return nil
		},
	}
	upCmd.Flags().String("track", "", "track ID (defaults to $TRACKS_ID)")
	register(upCmd)

	// tracks down <service> [--track <id>]
	downCmd := &cobra.Command{
		Use:   "down <service>",
		Short: "stop a running dev-server service for a track",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks down")
			if err != nil {
				return err
			}
			svcName := args[0]
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			err = cl.ServiceDownWithProgress(id, svcName, func(msg string) {
				fmt.Println(msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			return nil
		},
	}
	downCmd.Flags().String("track", "", "track ID (defaults to $TRACKS_ID)")
	register(downCmd)

	// tracks url <service> [--track <id>]
	urlCmd := &cobra.Command{
		Use:   "url <service>",
		Short: "print the URL for a running service",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks url")
			if err != nil {
				return err
			}
			svcName := args[0]
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			result, err := cl.Services(id)
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			port, portOK := result.Ports[svcName]
			if !portOK {
				return fmt.Errorf("service %q not found in track %s", svcName, id)
			}
			for _, ss := range result.Services {
				if ss.Name == svcName && !ss.Status.Live() {
					fmt.Fprintf(os.Stderr, "warning: service %s is not running (status: %s)\n", svcName, ss.Status)
				}
			}
			// Also show the stable proxy URL if this service has one active.
			if proxyStatus, err := cl.ProxyStatus(); err == nil {
				for _, p := range proxyStatus.Proxies {
					if p.ServiceName == svcName && p.Upstream != "" {
						fmt.Printf("stable:  http://localhost:%d\n", p.PublicPort)
					}
				}
			}
			fmt.Printf("track:   http://localhost:%d\n", port)
			return nil
		},
	}
	urlCmd.Flags().String("track", "", "track ID (defaults to $TRACKS_ID)")
	register(urlCmd)

	// tracks proxy [switch <service> [track-id]]
	//
	// tracks proxy          — show status of all stable-port proxies
	// tracks proxy switch <service> [track-id|off]
	//                       — flip a proxy's upstream to a track (or clear it)
	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "manage stable-port reverse proxies for dev-server services",
		Long: "Show and control the stable-port proxy that always listens on a fixed port " +
			"(e.g. :3000) and forwards to whichever track's service is currently active. " +
			"Your Wallet app stays pointed at the fixed port; you flip the upstream instead of patching manifests.",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			result, err := cl.ProxyStatus()
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			if len(result.Proxies) == 0 {
				fmt.Println("no proxy_port configured in any service (add proxy_port: <N> to a service in config.yaml)")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SERVICE\tFIXED PORT\tUPSTREAM\tACTIVE TRACK")
			for _, p := range result.Proxies {
				upstream := p.Upstream
				if upstream == "" {
					upstream = "(none — returns 503)"
				}
				trackID := p.ActiveTrackID
				if trackID == "" && p.Upstream != "" {
					trackID = "(unknown track)"
				}
				fmt.Fprintf(tw, "%s\t:%d\t%s\t%s\n", p.ServiceName, p.PublicPort, upstream, trackID)
			}
			return tw.Flush()
		},
	}

	proxySwitchCmd := &cobra.Command{
		Use:   "switch <service> [track-id|off]",
		Short: "switch a proxy's upstream to a track's service, or clear it",
		Long: "Set the active upstream for a service's stable-port proxy. " +
			"Passing a track ID routes the fixed port to that track's allocated service port. " +
			"Passing 'off' (or no argument) clears the upstream so the proxy returns 503.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			svcName := args[0]
			trackID := ""
			if len(args) == 2 {
				trackID = args[1]
			} else {
				// Default to current track if inside one.
				if id := os.Getenv("TRACKS_ID"); id != "" {
					trackID = id
				}
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl := daemon.NewClient(cfg)
			if err := cl.ProxySwitch(svcName, trackID); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			if trackID == "" || trackID == "off" {
				fmt.Printf("proxy for %s cleared (returning 503)\n", svcName)
			} else {
				fmt.Printf("proxy for %s → track %s\n", svcName, trackID)
			}
			return nil
		},
	}

	proxyCmd.AddCommand(proxySwitchCmd)
	register(proxyCmd)
}
