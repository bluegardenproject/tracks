// Package tui hosts the palette, form theme and small helpers shared
// by every tracks screen.
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
)

// EscQuitKeyMap returns a huh keymap identical to the default
// except that Esc — in addition to Ctrl-C — quits the form. The
// default huh keymap binds only Ctrl-C, which is unintuitive when
// every other CLI affordance in `tracks` (the menu, the dashboard)
// uses Esc to back out.
//
// Forms get it through RunForm, which also applies the tracks theme.
func EscQuitKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "back"),
	)
	return km
}
