package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// The tracks palette. The menu popup, the forms and the dashboard all
// draw from it so they read as one application rather than three.
var (
	ColorAccent    lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "30", Dark: "14"}
	ColorHighlight lipgloss.TerminalColor = lipgloss.Color("207")
	ColorInfo      lipgloss.TerminalColor = lipgloss.Color("12")
	ColorOK        lipgloss.TerminalColor = lipgloss.Color("10")
	ColorWarn      lipgloss.TerminalColor = lipgloss.Color("11")
	ColorFail      lipgloss.TerminalColor = lipgloss.Color("9")
	// ColorMuted is a mid-dark gray on light terminals, where ANSI 8
	// turns nearly invisible, and a lighter gray on dark ones.
	ColorMuted       lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "240", Dark: "245"}
	ColorFaint       lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "248", Dark: "240"}
	ColorBorder      lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	ColorSurface     lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "254", Dark: "234"}
	ColorText        lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	ColorSelectionFg lipgloss.TerminalColor = lipgloss.Color("15")
	ColorSelectionBg lipgloss.TerminalColor = lipgloss.Color("236")
)

// Theme is the huh theme for every tracks form, built from the palette.
func Theme() *huh.Theme {
	t := huh.ThemeBase()

	t.Focused.Base = t.Focused.Base.BorderForeground(ColorBorder)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(ColorAccent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(ColorAccent).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(ColorAccent)
	t.Focused.Description = t.Focused.Description.Foreground(ColorMuted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(ColorFail)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(ColorFail)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(ColorHighlight)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(ColorHighlight)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(ColorHighlight)
	t.Focused.Option = t.Focused.Option.Foreground(ColorText)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(ColorHighlight)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(ColorOK)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(ColorOK).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(ColorMuted).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(ColorText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Bold(true).
		Foreground(ColorSelectionFg).Background(ColorSelectionBg)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.
		Foreground(ColorMuted).Background(ColorSurface)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(ColorHighlight)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(ColorFaint)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(ColorAccent)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

// RunForm runs f with the tracks theme and the Esc-to-back keymap.
func RunForm(f *huh.Form) error {
	return f.WithTheme(Theme()).WithKeyMap(EscQuitKeyMap()).Run()
}
