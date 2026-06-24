package dashboard

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestBannerLayoutGeometry locks the banner's shape: bannerRows lines,
// all the same display width, every glyph 4 columns wide joined by a
// single space.
func TestBannerLayoutGeometry(t *testing.T) {
	lines := bannerLayout("TRACKS")
	if len(lines) != bannerRows {
		t.Fatalf("got %d lines, want %d", len(lines), bannerRows)
	}
	const glyphs = 6 // T R A C K S
	want := glyphs*4 + (glyphs - 1)
	for i, ln := range lines {
		if got := utf8.RuneCountInString(ln); got != want {
			t.Errorf("line %d width = %d, want %d (%q)", i, got, want, ln)
		}
	}
}

// TestBannerLayoutPreview prints the raw glyphs so a human can eyeball
// the letterforms with `go test -run Preview -v`.
func TestBannerLayoutPreview(t *testing.T) {
	t.Log("\n" + strings.Join(bannerLayout("TRACKS"), "\n"))
}

// TestBigBannerShape confirms the rendered banner still spans
// bannerRows lines after coloring. (Whether ANSI is emitted depends on
// the terminal profile, which is absent under `go test`, so color is
// covered by TestGradientAt instead.)
func TestBigBannerShape(t *testing.T) {
	if n := strings.Count(bigBanner("TRACKS"), "\n"); n != bannerRows-1 {
		t.Fatalf("got %d newlines, want %d", n, bannerRows-1)
	}
}

// TestBannerRowColors checks the banner paints one color per half-
// block line: six distinct colors, anchored to the first and last
// title stops at the ends.
func TestBannerRowColors(t *testing.T) {
	colors := bannerRowColors()
	if len(colors) != bannerLines {
		t.Fatalf("got %d colors, want %d", len(colors), bannerLines)
	}
	if got := string(colors[0]); got != "#FF10F0" {
		t.Errorf("top line = %s, want #FF10F0", got)
	}
	if got := string(colors[bannerLines-1]); got != "#00F0FF" {
		t.Errorf("bottom line = %s, want #00F0FF", got)
	}
	seen := map[string]int{}
	for i, c := range colors {
		seen[string(c)]++
		if seen[string(c)] > 1 {
			t.Errorf("color %s repeats (line %d); the six lines should be distinct", c, i)
		}
	}
}

// TestGradientAt pins the vertical gradient: the endpoints hit the
// first and last stops exactly, out-of-range t clamps, and a midpoint
// interpolates RGB between the bracketing stops.
func TestGradientAt(t *testing.T) {
	cases := []struct {
		t    float64
		want string
	}{
		{-1, "#FF10F0"},  // clamp low → first stop
		{0, "#FF10F0"},   // first stop
		{0.5, "#DF00FF"}, // halfway between #FF00FF and #BF00FF
		{1, "#00F0FF"},   // last stop
		{2, "#00F0FF"},   // clamp high → last stop
	}
	for _, c := range cases {
		if got := string(gradientAt(titleStops, c.t)); got != c.want {
			t.Errorf("gradientAt(t=%v) = %s, want %s", c.t, got, c.want)
		}
	}
}
