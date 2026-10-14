package trackwin

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
)

// SetComments shows c on window for the footer's button: the count, ""
// for none, and whether the prompt was sent.
func SetComments(t Tmux, window string, c actions.Comments) error {
	open, sent := "", ""
	if c.Open > 0 {
		open = strconv.Itoa(c.Open)
	}
	if c.Sent {
		sent = "1"
	}
	if err := t.SetWindowOption(window, actions.CommentsOption, open); err != nil {
		return err
	}
	return t.SetWindowOption(window, actions.SentOption, sent)
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
