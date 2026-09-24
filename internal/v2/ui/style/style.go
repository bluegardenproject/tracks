// Package style turns theme tokens into Lip Gloss colours for the
// terminal's background (dark or light).
package style

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// Palette resolves tokens of one theme for one background.
type Palette struct {
	theme theme.Theme
	dark  bool
}

// New returns a palette for t on a dark or light background.
func New(t theme.Theme, dark bool) Palette { return Palette{theme: t, dark: dark} }

// Color returns the colour for token.
func (p Palette) Color(token theme.Token) color.Color {
	return lipgloss.Color(p.theme.Value(token).For(p.dark))
}

// Dark reports whether the palette is for a dark background.
func (p Palette) Dark() bool { return p.dark }

// Theme is the palette's theme.
func (p Palette) Theme() theme.Theme { return p.theme }
