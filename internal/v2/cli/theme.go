package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/v2/footer"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

// themePath holds the session's applied theme, read by every process
// that draws.
func themePath(paths platform.Paths) string { return filepath.Join(paths.DataDir, "theme.yaml") }

// themeConfPath holds the tmux side of the applied theme.
func themeConfPath(paths platform.Paths) string { return filepath.Join(paths.DataDir, "theme.conf") }

// selfCommand runs this Tracks with profile's flags, quoted for a shell.
func selfCommand(profile platform.Profile) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find own binary: %w", err)
	}
	command := shellx.QuoteIfNeeded(self) + " --new-app"
	if profile == platform.Demo {
		command += " --demo"
	}
	return command, nil
}

// writeThemeConf renders t for tmux: colours and the footer rows.
func writeThemeConf(paths platform.Paths, t theme.Theme, dark bool, version, command string) error {
	systemCommand := command + " footer system"
	if !dark {
		systemCommand += " --light"
	}
	conf := tmux.ThemeConf{Colors: tmuxColors(t, dark), Footer: footer.Rows(t, dark, version, systemCommand)}
	if err := conf.Write(themeConfPath(paths)); err != nil {
		return fmt.Errorf("write tmux theme: %w", err)
	}
	return nil
}

// applyTheme makes t the running session's theme: saved for processes
// that start later, and reloaded into tmux.
func applyTheme(c *tmux.Client, paths platform.Paths, t theme.Theme, dark bool, version, command string) error {
	if err := t.Save(themePath(paths)); err != nil {
		return fmt.Errorf("save theme: %w", err)
	}
	if err := writeThemeConf(paths, t, dark, version, command); err != nil {
		return err
	}
	return c.SourceFile(themeConfPath(paths))
}

// tmuxColors picks the colours tmux draws pane borders, titles and the
// backgrounds with.
func tmuxColors(t theme.Theme, dark bool) tmux.Colors {
	c := func(token theme.Token) string { return t.Value(token).For(dark) }
	return tmux.Colors{
		Border:       c(theme.BorderDefault),
		BorderActive: c(theme.BorderFocus),
		Title:        c(theme.TextMuted),
		TitleActive:  c(theme.TextAccent),
		FooterBg:     c(theme.FooterBg),
		FooterFg:     c(theme.FooterText),
		Background:   c(theme.BgBase),
	}
}
