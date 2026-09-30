package cli

import (
	"context"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/ui/quickaccess"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/spf13/cobra"
)

// newQuickAccessCmd is what Ctrl+b q runs: Quick Access over client,
// then what was picked in it. A popup can't open another, so it opens
// them one after the other, each waiting for the last to close.
func newQuickAccessCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "quick-access <client>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			c := tmux.New(paths.TmuxSocket)
			if err := quickAccess(c, paths, args[0]); err != nil {
				return c.DisplayMessage("Couldn't open Quick Access: " + err.Error())
			}
			return nil
		},
	}
}

func quickAccess(c *tmux.Client, paths platform.Paths, client string) error {
	command, err := selfCommand()
	if err != nil {
		return err
	}
	width, height, err := c.ClientSize(client)
	if err != nil {
		return err
	}
	version, _ := tmux.InstalledVersion()
	border := tmux.PopupBorder(version)
	t, _ := loadTheme(paths)
	choice, err := os.CreateTemp("", "tracks-quick-access-*")
	if err != nil {
		return err
	}
	choice.Close()
	defer os.Remove(choice.Name())

	popup := tmux.Popup{
		Client:     client,
		Width:      strconv.Itoa(min(width, quickaccess.Width+border)),
		Height:     strconv.Itoa(min(height, quickaccess.Height()+border)),
		Command:    command + " popup quick-access " + shellx.Quote(choice.Name()),
		Background: t.Value(theme.OverlayBg),
	}
	if err := c.Popup(popup, version); err != nil {
		return err
	}
	picked, err := os.ReadFile(choice.Name())
	if err != nil {
		return err
	}
	switch strings.TrimSpace(string(picked)) {
	case quickaccess.NewTrack:
		return openNewTrack(c, paths, client, "")
	case quickaccess.TracksFilter:
		return openTracksFilter(c, paths, client)
	}
	return nil
}

// newPopupCmd holds what runs inside the popups.
func newPopupCmd(version string) *cobra.Command {
	cmd := &cobra.Command{Use: "popup", Hidden: true}
	cmd.AddCommand(&cobra.Command{
		Use:  "quick-access <choice-file>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t, _ := loadTheme(paths)
			done, err := runPopup(cmd.Context(), quickaccess.New(t))
			if err != nil {
				return err
			}
			var chosen string
			if m, ok := done.(quickaccess.Model); ok {
				chosen = m.Chosen()
			}
			return os.WriteFile(args[0], []byte(chosen), 0o600)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:  "tracks-filter <result-file>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tracksFilter(cmd.Context(), version, args[0])
		},
	})
	var draft string
	add := &cobra.Command{
		Use:  "add-track <client>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addTrack(cmd.Context(), version, args[0], draft)
		},
	}
	add.Flags().StringVar(&draft, "draft", "", "fill the form with this draft")
	cmd.AddCommand(add)
	return cmd
}

func runPopup(ctx context.Context, m tea.Model) (tea.Model, error) {
	done, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithColorProfile(style.Profile())).Run()
	if ctx.Err() != nil {
		return done, nil
	}
	return done, err
}

// repoNames are the Repositories tab's repos, in its order.
func repoNames(ctx context.Context, paths platform.Paths) ([]string, error) {
	db, err := store.Open(ctx, paths.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	all, err := db.Repos(ctx)
	names := make([]string, len(all))
	for i, r := range all {
		names[i] = r.Name
	}
	return names, err
}
