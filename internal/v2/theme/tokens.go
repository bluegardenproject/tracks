package theme

// Token names what a colour is for. App code uses tokens, never
// colour values.
type Token string

// Tokens. Adding one means adding it here, to All, and a value to every
// built-in theme; the tests fail otherwise.
const (
	TextDefault Token = "text.default"
	TextMuted   Token = "text.muted"
	TextFaint   Token = "text.faint"
	TextInverse Token = "text.inverse"
	TextAccent  Token = "text.accent"

	BgBase     Token = "bg.base"
	BgSurface  Token = "bg.surface"
	BgOverlay  Token = "bg.overlay"
	BgSelected Token = "bg.selected"
	BgHover    Token = "bg.hover"

	BorderDefault Token = "border.default"
	BorderFocus   Token = "border.focus"

	Accent    Token = "accent"
	Highlight Token = "highlight"

	StateSuccess Token = "state.success"
	StateWarning Token = "state.warning"
	StateDanger  Token = "state.danger"
	StateInfo    Token = "state.info"
)

// All lists every token in display order.
var All = []Token{
	TextDefault, TextMuted, TextFaint, TextInverse, TextAccent,
	BgBase, BgSurface, BgOverlay, BgSelected, BgHover,
	BorderDefault, BorderFocus,
	Accent, Highlight,
	StateSuccess, StateWarning, StateDanger, StateInfo,
}
