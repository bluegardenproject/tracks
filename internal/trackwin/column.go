package trackwin

import "github.com/bluegardenproject/tracks/internal/tmux"

// Column returns the panes of window's right column, top to bottom:
// every pane but the agent's.
func Column(t Tmux, window string) ([]tmux.Pane, error) {
	panes, err := t.ListPanes(window)
	if err != nil {
		return nil, err
	}
	_, column := layout(panes)
	return column, nil
}

// Even gives every pane in window's right column the same height, as
// adding a pane does; closing one leaves its rows to a neighbour.
func Even(t Tmux, window string) error { return even(t, window) }
