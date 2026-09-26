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
	BorderAccent  Token = "border.accent"

	// States, four each: text on the window, a soft background with
	// text readable on it, and a solid badge (bg.accent) with the text
	// drawn on it (text.accent).
	StateSuccessText       Token = "state.success.text"
	StateSuccessTextAccent Token = "state.success.text.accent"
	StateSuccessBg         Token = "state.success.bg"
	StateSuccessBgAccent   Token = "state.success.bg.accent"
	StateWarningText       Token = "state.warning.text"
	StateWarningTextAccent Token = "state.warning.text.accent"
	StateWarningBg         Token = "state.warning.bg"
	StateWarningBgAccent   Token = "state.warning.bg.accent"
	StateDangerText        Token = "state.danger.text"
	StateDangerTextAccent  Token = "state.danger.text.accent"
	StateDangerBg          Token = "state.danger.bg"
	StateDangerBgAccent    Token = "state.danger.bg.accent"
	StateInfoText          Token = "state.info.text"
	StateInfoTextAccent    Token = "state.info.text.accent"
	StateInfoBg            Token = "state.info.bg"
	StateInfoBgAccent      Token = "state.info.bg.accent"

	// The footer has its own set, so it can stand apart from the windows.
	FooterBg         Token = "footer.bg"
	FooterText       Token = "footer.text"
	FooterMuted      Token = "footer.muted"
	FooterFaint      Token = "footer.faint"
	FooterActiveBg   Token = "footer.active.bg"
	FooterActiveText Token = "footer.active.text"

	// The Tracks banner fades from top to bottom.
	BannerTop    Token = "banner.top"
	BannerBottom Token = "banner.bottom"

	// Tabs of the Tracks window.
	TabText         Token = "tab.text"
	TabBorder       Token = "tab.border"
	TabActiveBg     Token = "tab.active.bg"
	TabActiveText   Token = "tab.active.text"
	TabActiveBorder Token = "tab.active.border"

	// Tables, such as Station's track list.
	TableTextDefault Token = "table.text.default"
	TableTextMuted   Token = "table.text.muted"
	TableTextAccent  Token = "table.text.accent"
	TableTextFaint   Token = "table.text.faint"
	TableBgHighlight Token = "table.bg.highlight" // the row under the mouse
	TableBgSelected  Token = "table.bg.selected"

	// Text inputs.
	InputBg Token = "input.bg"

	// Buttons: default ones, the focused or primary one (accent), and
	// ones that destroy something (danger).
	ButtonBgDefault     Token = "button.bg.default"
	ButtonBgHover       Token = "button.bg.hover"
	ButtonBgDanger      Token = "button.bg.danger"
	ButtonBgDangerHover Token = "button.bg.danger.hover"
	ButtonBgAccent      Token = "button.bg.accent"
	ButtonBgAccentHover Token = "button.bg.accent.hover"
	ButtonTextDefault   Token = "button.text.default"
	ButtonTextDanger    Token = "button.text.danger"
	ButtonTextAccent    Token = "button.text.accent"

	// List items, such as the Settings sidebar's sections: the one
	// under the mouse (hover) and the chosen one (active).
	ListItemBgDefault   Token = "listItem.bg.default"
	ListItemTextDefault Token = "listItem.text.default"
	ListItemBgHover     Token = "listItem.bg.hover"
	ListItemTextHover   Token = "listItem.text.hover"
	ListItemBgActive    Token = "listItem.bg.active"
	ListItemTextActive  Token = "listItem.text.active"
)

// All lists every token in display order.
var All = []Token{
	TextDefault, TextMuted, TextFaint, TextInverse, TextAccent,
	BgBase, BgSurface, BgOverlay, BgSelected, BgHover,
	BorderDefault, BorderFocus, BorderAccent,
	StateSuccessText, StateSuccessTextAccent, StateSuccessBg, StateSuccessBgAccent,
	StateWarningText, StateWarningTextAccent, StateWarningBg, StateWarningBgAccent,
	StateDangerText, StateDangerTextAccent, StateDangerBg, StateDangerBgAccent,
	StateInfoText, StateInfoTextAccent, StateInfoBg, StateInfoBgAccent,
	FooterBg, FooterText, FooterMuted, FooterFaint, FooterActiveBg, FooterActiveText,
	BannerTop, BannerBottom,
	TabText, TabBorder, TabActiveBg, TabActiveText, TabActiveBorder,
	TableTextDefault, TableTextMuted, TableTextAccent, TableTextFaint, TableBgHighlight, TableBgSelected,
	InputBg,
	ButtonBgDefault, ButtonBgHover, ButtonBgDanger, ButtonBgDangerHover, ButtonBgAccent, ButtonBgAccentHover,
	ButtonTextDefault, ButtonTextDanger, ButtonTextAccent,
	ListItemBgDefault, ListItemTextDefault, ListItemBgHover, ListItemTextHover, ListItemBgActive, ListItemTextActive,
}
