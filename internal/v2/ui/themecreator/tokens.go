package themecreator

import (
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

// Text and background tokens outside the text. and bg. groups.
var (
	textTokens = map[theme.Token]bool{
		theme.FooterText: true, theme.FooterMuted: true, theme.FooterFaint: true, theme.FooterActiveText: true,
		theme.TabText: true, theme.TabActiveText: true,
	}
	backgroundTokens = map[theme.Token]bool{theme.FooterBg: true, theme.FooterActiveBg: true, theme.TabActiveBg: true}
)

func kindOf(token theme.Token) kind {
	switch {
	case strings.HasPrefix(string(token), "text."), strings.HasPrefix(string(token), "table.text."), textTokens[token]:
		return kindText
	case strings.HasPrefix(string(token), "bg."), strings.HasPrefix(string(token), "table.bg."), backgroundTokens[token]:
		return kindBackground
	default:
		return kindColour
	}
}

// backgroundOf is what a text token is previewed on.
func backgroundOf(token theme.Token) theme.Token {
	switch {
	case token == theme.TextInverse:
		return theme.Accent
	case token == theme.FooterActiveText:
		return theme.FooterActiveBg
	case token == theme.TabActiveText:
		return theme.TabActiveBg
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
