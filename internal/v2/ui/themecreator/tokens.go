package themecreator

import (
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// kind decides how a token is previewed.
type kind int

const (
	kindText kind = iota // sample text on its background
	kindBackground
	kindColour // a block and sample text
)

// textTokens are text tokens without "text" in their name.
var textTokens = map[theme.Token]bool{theme.FooterMuted: true, theme.FooterFaint: true}

// kindOf reads a token's kind from its name: a "text" part makes it
// text, a "bg" part a background.
func kindOf(token theme.Token) kind {
	parts := strings.Split(string(token), ".")
	switch {
	case slices.Contains(parts, "text"), textTokens[token]:
		return kindText
	case slices.Contains(parts, "bg"):
		return kindBackground
	default:
		return kindColour
	}
}

// backgroundOf is what a text token is previewed on.
func backgroundOf(token theme.Token) theme.Token {
	switch {
	case token == theme.TextInverse:
		return theme.ButtonBgAccent
	case token == theme.FooterActiveText:
		return theme.FooterActiveBg
	case token == theme.TabActiveText:
		return theme.TabActiveBg
	case strings.HasPrefix(string(token), "state."), strings.HasPrefix(string(token), "listItem."):
		return theme.Token(strings.Replace(string(token), ".text", ".bg", 1))
	case token == theme.ButtonTextDefault:
		return theme.ButtonBgDefault
	case token == theme.ButtonTextDanger:
		return theme.ButtonBgDanger
	case token == theme.ButtonTextAccent:
		return theme.ButtonBgAccent
	case strings.HasPrefix(string(token), "footer."):
		return theme.FooterBg
	default:
		return theme.BgBase
	}
}

// groupOf names a token's section: the part before the first dot.
func groupOf(token theme.Token) string {
	if g, _, ok := strings.Cut(string(token), "."); ok {
		return g
	}
	return "emphasis"
}

// layout places the tokens in list lines, with a blank line and a group
// name before each group. It returns the line of every token.
func layout() (lines int, tokenLine []int) {
	group := ""
	for _, token := range theme.All {
		if g := groupOf(token); g != group {
			if group != "" {
				lines++
			}
			lines++ // group name
			group = g
		}
		tokenLine = append(tokenLine, lines)
		lines++
	}
	return lines, tokenLine
}
