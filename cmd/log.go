package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "log <track-id>",
		Short: "tail a track's filtered log (used by tmux track windows)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			t, found, err := cl.Get(args[0])
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("track %s not found", args[0])
			}
			fmt.Printf("=== track %s (%s) ===\n", t.ID, t.Branch)
			fmt.Printf("repos: ")
			for i, r := range t.Repos {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Print(r.Name)
			}
			fmt.Println()
			fmt.Println("--- live output below ---")

			events := make(chan claude.Event, 64)
			go func() {
				_ = claude.TailLog(c.Context(), t.LogPath, events)
				close(events)
			}()
			for ev := range events {
				renderEvent(ev)
			}
			return nil
		},
	})
}

// renderEvent prints one event in a human-friendly form. Mirrors
// the "minimal output" philosophy from the plan: tool calls + key
// markers + assistant prose, no raw JSON.
func renderEvent(ev claude.Event) {
	switch e := ev.(type) {
	case claude.AssistantText:
		fmt.Println(e.Text)
	case claude.ToolUse:
		// Best-effort short input preview.
		preview := ""
		if len(e.Input) > 0 && len(e.Input) < 120 {
			var m map[string]any
			if err := json.Unmarshal(e.Input, &m); err == nil {
				parts := []string{}
				for k, v := range m {
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				}
				preview = " " + strings.Join(parts, " ")
			}
		}
		fmt.Fprintf(os.Stdout, "[tool] %s%s\n", e.Name, preview)
	case claude.PRMarker:
		if e.URL == "" {
			fmt.Println("[no PR opened]")
		} else {
			fmt.Printf("[PR] %s\n", e.URL)
		}
	}
}

// silence unused context import in some builds
var _ = context.Background
