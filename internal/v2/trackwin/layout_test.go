package trackwin

import (
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

func TestLayoutUsesRoles(t *testing.T) {
	panes := []tmux.Pane{
		{ID: "%3", Role: RoleDevServer, Top: 12},
		{ID: "%1", Role: RoleAgent},
		{ID: "%2", Role: RoleTerminal, Top: 1},
		{ID: "%4", Role: ""},
	}
	agent, column := layout(panes)
	if agent == nil || agent.ID != "%1" {
		t.Fatalf("agent = %v, want %%1", agent)
	}
	var ids []string
	for _, p := range column {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids, []string{"%2", "%3"}) {
		t.Errorf("column = %v, want [%%2 %%3] (top to bottom, unknown roles ignored)", ids)
	}
}

func TestColumnHeights(t *testing.T) {
	tests := []struct {
		name   string
		column []tmux.Pane
		want   []int
	}{
		{"none", nil, nil},
		{"one", []tmux.Pane{{Top: 1, Height: 40}}, []int{40}},
		{"two uneven", []tmux.Pane{{Top: 1, Height: 30}, {Top: 32, Height: 9}}, []int{20, 19}},
		{"three", []tmux.Pane{{Top: 1, Height: 10}, {Top: 12, Height: 10}, {Top: 23, Height: 18}}, []int{13, 13, 12}},
	}
	for _, tt := range tests {
		if got := columnHeights(tt.column); !slices.Equal(got, tt.want) {
			t.Errorf("%s: columnHeights = %v, want %v", tt.name, got, tt.want)
		}
	}
}
