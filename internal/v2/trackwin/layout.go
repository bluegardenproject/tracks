package trackwin

import (
	"slices"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

// layout finds the agent pane and the right column's panes, top to
// bottom. Roles decide, not geometry, so a pane the user moved or
// resized is still found.
func layout(panes []tmux.Pane) (agent *tmux.Pane, column []tmux.Pane) {
	for i, p := range panes {
		switch p.Role {
		case RoleAgent:
			agent = &panes[i]
		case RoleTerminal, RoleDevServer:
			column = append(column, p)
		}
	}
	slices.SortFunc(column, func(a, b tmux.Pane) int { return a.Top - b.Top })
	return agent, column
}

// columnHeights splits the column's height evenly between its panes.
// Every pane but the first loses one row to the border above it.
func columnHeights(column []tmux.Pane) []int {
	if len(column) == 0 {
		return nil
	}
	last := column[len(column)-1]
	total := last.Top + last.Height - column[0].Top // includes the borders between panes
	n := len(column)
	content := total - (n - 1)
	heights := make([]int, n)
	for i := range heights {
		heights[i] = content / n
		if i < content%n {
			heights[i]++
		}
	}
	return heights
}
