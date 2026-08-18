package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
		Use:   "up [service]",
		Short: "start a dev-server service for a track (omit the name to start all of them)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks up")
			if err != nil {
				return err
			}
			svcName := ""
			if len(args) == 1 {
				svcName = args[0]
			}
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
			if svcName == "" {
				fmt.Println("services launching in their panes — deps install + server start run there; not confirmed up yet")
				fmt.Println("run `tracks services` to see status and log paths, and tail a log to confirm readiness")
				return nil
			}
			fmt.Printf("%s launching in a pane on :%d — deps install + server start run there; not confirmed up yet\n", svcName, result.Port)
			fmt.Printf("tail this log to confirm it actually came up: %s\n", result.LogPath)
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
			// Also show the stable proxy URL if a port forwards to this service.
			if proxyStatus, err := cl.ProxyStatus(); err == nil {
				for _, p := range proxyStatus.Proxies {
					if p.ActiveService == svcName && p.ActiveTrackID == id {
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

	// tracks proxy                            — show every defined stable port
	// tracks proxy add <port> [--bind-all]    — define a new stable port
	// tracks proxy rm <port>                  — delete a stable port
	// tracks proxy switch <port> [track|off] [service]
	//                                         — link a port to a running server
	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "manage stable-port reverse proxies for dev servers",
		Long: "Show and control the stable ports that always listen on a fixed port " +
			"(e.g. :3000) and forward to whichever running dev server you link them to. " +
			"Your app stays pointed at the fixed port; you flip the upstream instead of patching manifests. " +
			"Ports are user-defined runtime state — add and remove them here.",
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
				fmt.Println("no stable ports defined — add one with `tracks proxy add <port>`")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PORT\tUPSTREAM\tTRACK\tSERVICE\tBIND")
			for _, p := range result.Proxies {
				upstream := p.Upstream
				if upstream == "" {
					upstream = "(none — returns 503)"
				}
				bind := "loopback"
				if p.BindAll {
					bind = "all"
				}
				fmt.Fprintf(tw, ":%d\t%s\t%s\t%s\t%s\n", p.PublicPort, upstream, p.ActiveTrackID, p.ActiveService, bind)
			}
			return tw.Flush()
		},
	}

	var addBindAll bool
	proxyAddCmd := &cobra.Command{
		Use:   "add <port>",
		Short: "define a new stable port",
		Long: "Define a stable port. It does not bind until you link it to a running dev " +
			"server with `tracks proxy switch`. Pass --bind-all to expose it on every " +
			"network interface (needed for a physical device; off by default).",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := daemon.NewClient(cfg).ProxyAdd(port, addBindAll); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Printf("stable port :%d defined — link it with `tracks proxy switch %d <track>`\n", port, port)
			return nil
		},
	}
	proxyAddCmd.Flags().BoolVar(&addBindAll, "bind-all", false, "expose the port on every network interface (default loopback only)")

	proxyRmCmd := &cobra.Command{
		Use:     "rm <port>",
		Aliases: []string{"remove"},
		Short:   "delete a stable port",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := daemon.NewClient(cfg).ProxyRemove(port); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Printf("stable port :%d removed\n", port)
			return nil
		},
	}

	proxySwitchCmd := &cobra.Command{
		Use:   "switch <port> [track-id|off] [service]",
		Short: "link a stable port to a running server, or clear it",
		Long: "Point a stable port at a running dev server. Passing a track ID routes the " +
			"fixed port to that track's service (any service, of any name). When the track " +
			"runs a single service the name may be omitted. Passing 'off' (or no track) " +
			"clears the port so it returns 503.",
		Args: cobra.RangeArgs(1, 3),
		RunE: func(c *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			trackID := ""
			service := ""
			if len(args) >= 2 {
				trackID = args[1]
			} else if id := os.Getenv("TRACKS_ID"); id != "" {
				trackID = id // default to current track if inside one
			}
			if len(args) == 3 {
				service = args[2]
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := daemon.NewClient(cfg).ProxySwitch(port, trackID, service); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			if trackID == "" || trackID == "off" {
				fmt.Printf("proxy :%d cleared (returning 503)\n", port)
			} else {
				fmt.Printf("proxy :%d → track %s\n", port, trackID)
			}
			return nil
		},
	}

	proxyCmd.AddCommand(proxyAddCmd, proxyRmCmd, proxySwitchCmd)
	register(proxyCmd)
}

// parsePort parses a port argument, tolerating a leading colon (":3000").
func parsePort(s string) (int, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), ":")
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q: want a number between 1 and 65535", s)
	}
	return p, nil
}
