package cursor

import "github.com/bluegardenproject/tracks/internal/agents"

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
		if print {
			line.Arg(r.Prompt)
		}
		return string(line.Build())
	}
	report := string(agents.NewLine(r.Report).Build())
	return turn(true) + " | tee " + report + "\n" + r.Reply + "\nexec " + turn(false)
}
