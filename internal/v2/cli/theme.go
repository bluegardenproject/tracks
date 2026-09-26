package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/v2/footer"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// loadTheme returns the chosen theme, read by every process that draws.
// It falls back to the default theme, along with the error, when the
// choice can't be read.
func loadTheme(paths platform.Paths) (theme.Theme, error) {
	s, err := settings.Load(paths.Settings)
	t, terr := theme.Load(paths.ThemesDir, s.Theme)
	return t, errors.Join(err, terr)
}

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
func writeThemeConf(paths platform.Paths, t theme.Theme, version, command string) error {
	conf := tmux.ThemeConf{Colors: tmuxColors(t), Footer: footer.Rows(t, version, command+" footer system")}
	if err := conf.Write(themeConfPath(paths)); err != nil {
		return fmt.Errorf("write tmux theme: %w", err)
	}
	return nil
}

// tmuxColors picks the colours tmux draws pane borders, titles and the
// backgrounds with.
func tmuxColors(t theme.Theme) tmux.Colors {
	return tmux.Colors{
		Border:       t.Value(theme.BorderDefault),
		BorderActive: t.Value(theme.BorderFocus),
		Title:        t.Value(theme.TextMuted),
		TitleActive:  t.Value(theme.TextAccent),
		FooterBg:     t.Value(theme.FooterBg),
		FooterFg:     t.Value(theme.FooterText),
		Background:   t.Value(theme.BgBase),
	}
}

// themes lists, chooses and saves themes for the Settings tab. Choosing
// or saving the chosen theme reloads it into tmux.
type themes struct {
	c                *tmux.Client
	paths            platform.Paths
	version, command string
}

var _ source.Themes = themes{}

func (s themes) List() ([]theme.Entry, error) { return theme.List(s.paths.ThemesDir) }

func (s themes) Choose(id string) (theme.Theme, error) {
	t, err := theme.Load(s.paths.ThemesDir, id)
	if err != nil {
		return theme.Theme{}, err
	}
	st, err := settings.Load(s.paths.Settings)
	if err != nil {
		return theme.Theme{}, err
	}
	st.Theme = id
	if err := settings.Save(s.paths.Settings, st); err != nil {
		return theme.Theme{}, err
	}
	return t, s.apply(t)
}

func (s themes) Save(t theme.Theme) error {
	if err := t.Save(s.paths.ThemesDir); err != nil {
		return err
	}
	st, err := settings.Load(s.paths.Settings)
	if err != nil || st.Theme != t.ID {
		return nil
	}
	return s.apply(t)
}

func (s themes) Create(name string, t theme.Theme) (theme.Theme, error) {
	return theme.Create(s.paths.ThemesDir, name, t)
}

func (s themes) apply(t theme.Theme) error {
	if err := writeThemeConf(s.paths, t, s.version, s.command); err != nil {
		return err
	}
	return s.c.SourceFile(themeConfPath(s.paths))
}
