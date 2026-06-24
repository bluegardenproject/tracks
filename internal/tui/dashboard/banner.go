package dashboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// titleStops are the anchor colors of the banner's vertical gradient:
// neon pink fading through magenta and purple down to cyan, matching
// the github-butler house style. They're sampled into one color per
// half-block line (see bannerRowColors) so the word fades top-to-
// bottom across all six horizontal lines.
var titleStops = []lipgloss.Color{
	lipgloss.Color("#FF10F0"), // neon pink
	lipgloss.Color("#FF00FF"), // magenta
	lipgloss.Color("#BF00FF"), // purple
	lipgloss.Color("#00F0FF"), // cyan
}

// bannerRows is the height of the block-letter banner in text rows.
// Each glyph is drawn on this many rows, and because the glyphs use
// half-block characters every text row is two horizontal "pixel"
// lines tall — so the banner reads as bannerLines lines vertically.
const bannerRows = 3

// bannerLines is the banner's height in half-block lines: two per
// text row. This is the number of distinct colors the vertical
// gradient paints.
const bannerLines = 2 * bannerRows

// bigLetters is a bannerRows-tall, 4-column block font covering the
// characters in "TRACKS". Glyphs use Unicode block and quadrant
// characters (U+2580…U+259F) so that within three text rows we can
// draw at a higher effective resolution — half-blocks (▀ ▄) give
// single-pixel-tall strokes and the corner quadrants (▟ ▙ ▜ ▛) give
// rounded outer corners. Unknown runes fall back to a solid block so
// the banner still renders rather than disappearing.
var bigLetters = map[rune][bannerRows]string{
	'T': {"████", " ██ ", " ██ "},
	'R': {"██▀▙", "██▄▛", "█ ▜▄"},
	'A': {"▟██▙", "████", "█  █"},
	'C': {"▟██▙", "██  ", "▜██▛"},
	'K': {"██▟█", "███ ", "██▜█"},
	'S': {"█▀▀▀", "▀▀▀█", "████"},
	' ': {"    ", "    ", "    "},
}

// bannerLayout assembles text into bannerRows uncolored lines of
// block-letter glyphs, one column of space between glyphs. Separated
// from coloring so the geometry can be tested without ANSI noise.
func bannerLayout(text string) []string {
	runes := []rune(strings.ToUpper(text))

	var rows [bannerRows]strings.Builder
	for i, r := range runes {
		glyph, ok := bigLetters[r]
		if !ok {
			glyph = [bannerRows]string{"████", "████", "████"}
		}
		for row := 0; row < bannerRows; row++ {
			rows[row].WriteString(glyph[row])
			if i < len(runes)-1 {
				rows[row].WriteString(" ") // single-column gap between glyphs
			}
		}
	}

	lines := make([]string, bannerRows)
	for row := 0; row < bannerRows; row++ {
		lines[row] = rows[row].String()
	}
	return lines
}

// bannerRowColors samples one color per half-block line of the
// banner, evenly spaced along the title stops — six colors for the
// six horizontal lines, fading top to bottom.
func bannerRowColors() [bannerLines]lipgloss.Color {
	var c [bannerLines]lipgloss.Color
	for i := range c {
		c[i] = gradientAt(titleStops, float64(i)/float64(bannerLines-1))
	}
	return c
}

// bigBanner renders text as a block-letter banner with a six-color
// vertical gradient. Each text row spans two gradient lines (its
// upper and lower half), so a solid cell is drawn as an upper-half
// block with the top color in the foreground and the bottom color in
// the background — letting one character cell show two of the six
// colors. The returned string already contains its own newlines.
func bigBanner(text string) string {
	colors := bannerRowColors()
	lines := bannerLayout(text)

	var out strings.Builder
	for row, line := range lines {
		top, bottom := colors[2*row], colors[2*row+1]
		for _, ch := range line {
			out.WriteString(styleCell(ch, top, bottom))
		}
		if row < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// styleCell colors one block-font character so its upper half takes
// the top color and its lower half the bottom color.
//
//   - Full blocks become an upper-half block (▀) with fg=top, bg=
//     bottom, painting the cell's two halves in two colors.
//   - Half blocks (▀ ▄) only occupy one half, so they take just that
//     half's color.
//   - Corner quadrants can't be split across two foreground colors,
//     so they take the color of whichever half holds more of the
//     glyph; the rounded shape is preserved.
//   - Spaces stay uncolored so gaps show the terminal background.
func styleCell(ch rune, top, bottom lipgloss.Color) string {
	bold := lipgloss.NewStyle().Bold(true)
	switch ch {
	case ' ':
		return " "
	case '█':
		return bold.Foreground(top).Background(bottom).Render("▀")
	case '▀':
		return bold.Foreground(top).Render("▀")
	case '▄':
		return bold.Foreground(bottom).Render("▄")
	case '▟', '▙': // three quadrants, weighted to the lower half
		return bold.Foreground(bottom).Render(string(ch))
	case '▜', '▛': // three quadrants, weighted to the upper half
		return bold.Foreground(top).Render(string(ch))
	default:
		return bold.Foreground(top).Render(string(ch))
	}
}

// gradientAt returns the color at fraction t (clamped to [0,1]) along
// stops, linearly interpolating RGB between the two bracketing stops.
func gradientAt(stops []lipgloss.Color, t float64) lipgloss.Color {
	switch {
	case len(stops) == 0:
		return lipgloss.Color("")
	case len(stops) == 1 || t <= 0:
		return stops[0]
	case t >= 1:
		return stops[len(stops)-1]
	}
	segments := len(stops) - 1
	seg := int(t * float64(segments))
	if seg >= segments {
		seg = segments - 1
	}
	localT := t*float64(segments) - float64(seg)
	a := hexToRGB(string(stops[seg]))
	b := hexToRGB(string(stops[seg+1]))
	c := [3]int{
		lerp(a[0], b[0], localT),
		lerp(a[1], b[1], localT),
		lerp(a[2], b[2], localT),
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", c[0], c[1], c[2]))
}

func lerp(a, b int, t float64) int {
	return int(float64(a) + (float64(b)-float64(a))*t)
}

func hexToRGB(h string) [3]int {
	if len(h) != 7 || h[0] != '#' {
		return [3]int{255, 255, 255}
	}
	r, _ := strconv.ParseInt(h[1:3], 16, 0)
	g, _ := strconv.ParseInt(h[3:5], 16, 0)
	b, _ := strconv.ParseInt(h[5:7], 16, 0)
	return [3]int{int(r), int(g), int(b)}
}
