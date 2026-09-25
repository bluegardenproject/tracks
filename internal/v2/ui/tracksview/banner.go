package tracksview

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// logo is "TRACKS" in a slanted ASCII font.
var logo = []string{
	`  __________  ___   ________ _______`,
	` /_  __/ __ \/   | / ____/ //_/ ___/`,
	`  / / / /_/ / /| |/ /   / ,<  \__ \ `,
	` / / / _, _/ ___ / /___/ /| |___/ / `,
	`/_/ /_/ |_/_/  |_\____/_/ |_/____/  `,
}

// bannerRows is the logo's height.
const bannerRows = 5

// bannerWidth is the logo's width in cells.
const bannerWidth = 36

// banner renders the logo, one colour per line, fading from banner.top
// to banner.bottom.
func (m Model) banner() []string {
	colors := lipgloss.Blend1D(bannerRows, m.palette.Color(theme.BannerTop), m.palette.Color(theme.BannerBottom))
	lines := make([]string, bannerRows)
	for i, l := range logo {
		lines[i] = lipgloss.NewStyle().Bold(true).Foreground(colors[i]).Render(l)
	}
	return lines
}
