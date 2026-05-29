// Package tui hosts small helpers shared across the
// huh-based menu/settings/newtrack packages.
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
// Use:
//
//	form := huh.NewForm(...).WithKeyMap(tui.EscQuitKeyMap())
func EscQuitKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "back"),
	)
	return km
}
