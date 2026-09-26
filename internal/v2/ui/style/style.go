// Package style turns theme tokens into Lip Gloss colours.
package style

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// Palette resolves the tokens of one theme.
type Palette struct {
	theme theme.Theme
}

// New returns a palette for t.
func New(t theme.Theme) Palette { return Palette{theme: t} }

// Color returns the colour for token.
func (p Palette) Color(token theme.Token) color.Color {
	return lipgloss.Color(p.theme.Value(token))
}

// Theme is the palette's theme.
func (p Palette) Theme() theme.Theme { return p.theme }
