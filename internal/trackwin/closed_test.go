package trackwin_test

import (
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// vanishing is a tmux whose new panes end at once: labelling one fails,
// and it's no longer listed.
type vanishing struct{ trackwin.Tmux }

func (vanishing) ListPanes(string) ([]tmux.Pane, error) {
	return []tmux.Pane{{ID: "%1", Role: trackwin.RoleAgent, Height: 40}}, nil
}

func (vanishing) SplitPane(string, tmux.Split, int, string, string) (string, error) { return "%9", nil }

func (vanishing) SetPaneOption(pane, _, _ string) error {
	if pane == "%9" {
		return errors.New("no such pane: %9")
	}
	return nil
}

func (vanishing) ResizePaneHeight(string, int) error { return nil }

func (vanishing) WindowOption(string, string) (string, error) { return "/tmp", nil }

// TestAPaneThatClosedAtOnce: a setup fast enough to finish before its
// pane is labelled isn't an error; a terminal's pane closing is.
func TestAPaneThatClosedAtOnce(t *testing.T) {
	if _, err := trackwin.AddSetup(vanishing{}, "@1", "/tmp", trackwin.Process{Title: "setup · api", Command: "true"}); err != nil {
		t.Errorf("AddSetup: %v; want a setup that already finished taken as started", err)
	}
	if _, err := trackwin.AddTerminal(vanishing{}, "@1"); !errors.Is(err, trackwin.ErrPaneClosed) {
		t.Errorf("a terminal whose pane closed at once: %v; want ErrPaneClosed", err)
	}
}
