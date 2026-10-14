package trackwin

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
)

// SetButtons shows buttons, a tmux format, on window for the footer;
// "" shows none.
func SetButtons(t Tmux, window, buttons string) error {
	return t.SetWindowOption(window, actions.ButtonsOption, buttons)
}

// InfoOption holds what the footer's top row shows of the track while
// it has no buttons.
const InfoOption = "@tracks_info"

// SetInfo shows text on window for the footer, escaped so tmux draws it
// as it is.
func SetInfo(t Tmux, window, text string) error {
	return t.SetWindowOption(window, InfoOption, strings.ReplaceAll(text, "#", "##"))
}

// submitDelay parts the typed prompt from its Enter: the agent CLIs
// take keys that arrive together for a paste, in which Enter is a new
// line.
var submitDelay = 300 * time.Millisecond

// Prompt types text into window's agent pane and presses Enter. An
// agent at work queues it for when its turn ends.
func Prompt(t Tmux, window, text string) error {
	panes, err := t.ListPanes(window)
	if err != nil {
		return err
	}
	agent, _ := layout(panes)
	if agent == nil {
		return fmt.Errorf("window %s has no agent pane", window)
	}
	if err := t.SendText(agent.ID, text); err != nil {
		return err
	}
	time.Sleep(submitDelay)
	return t.SendKey(agent.ID, "Enter")
}
