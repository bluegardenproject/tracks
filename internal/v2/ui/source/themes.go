package source

import "github.com/bluegardenproject/tracks/internal/v2/theme"

// Themes lists, chooses and saves themes.
type Themes interface {
	// List returns every theme, built-ins first; invalid files carry
	// the reason.
	List() ([]theme.Entry, error)
	// Choose makes the theme id the one in use, everywhere.
	Choose(id string) (theme.Theme, error)
	// Save writes a user theme over its file; the theme in use is
	// reapplied.
	Save(t theme.Theme) error
	// Create writes t as a new theme called name.
	Create(name string, t theme.Theme) (theme.Theme, error)
}
