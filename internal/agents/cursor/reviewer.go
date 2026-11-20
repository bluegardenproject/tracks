package cursor

import (
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/shellx"
)

// Reviewer is how a track's Cursor reviewer runs in its pane.
type Reviewer struct {
	Program, Model string
	// Chat is the chat both turns share, from CreateChat.
	Chat string
	// Workspace is the worktree under review.
	Workspace string
	// Prompt is its first message, Report the file its report goes to,
	// and Reply the command that hands that file to Tracks.
	Prompt, Report, Reply string
	// Status is shown in the pane as the print-mode turn starts; "" for
	// none.
	Status string
	// Stream is the command that shows the turn's stream as it comes and
	// writes its final text to the file after its --out; "" shows
	// nothing until the turn is done.
	Stream string
}

// Script is the pane's shell script. Cursor's hooks don't carry its
// replies' text in the interactive CLI, so the review is one turn in
// print mode, written to Report and shown, and handed over with Reply;
// the same chat then goes on interactively, for questions. Both turns
// run in ask mode: they read, and run and write nothing. --trust trusts
// the track's own worktree, which Tracks made, since a hidden pane can't
// answer Cursor's trust question.
func (r Reviewer) Script() string {
	turn := func(print bool) string {
		line := agents.NewLine(r.Program)
		if print {
			line.Flag("-p")
		}
		line.Set("--resume", r.Chat)
		line.Set("--mode", "ask")
		line.Flag("--trust")
		line.Set("--workspace", r.Workspace)
		line.SetIf("--model", r.Model)
		if print && r.Stream != "" {
			line.Set("--output-format", "stream-json")
			line.Flag("--stream-partial-output")
		}
		if print {
			line.Arg(r.Prompt)
		}
		return string(line.Build())
	}
	report := string(agents.NewLine(r.Report).Build())
	status := ""
	if r.Status != "" {
		status = "printf '%s\\n\\n' " + shellx.Quote(r.Status) + "\n"
	}
	keep := " | tee " + report
	if r.Stream != "" {
		keep = " | " + r.Stream + " --out " + report
	}
	return status + turn(true) + keep + "\n" + r.Reply + "\nexec " + turn(false)
}
