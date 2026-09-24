package cli

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

func TestPlanStartup(t *testing.T) {
	tests := []struct {
		loc    tmux.Location
		exists bool
		want   startAction
	}{
		{tmux.Outside, false, startCreate},
		{tmux.Outside, true, startAttach},
		{tmux.OwnServer, true, startSelect},
		{tmux.OtherServer, false, startRefuse},
		{tmux.OtherServer, true, startRefuse},
	}
	for _, tt := range tests {
		if got := planStartup(tt.loc, tt.exists); got != tt.want {
			t.Errorf("planStartup(%v, %v) = %v, want %v", tt.loc, tt.exists, got, tt.want)
		}
	}
}
