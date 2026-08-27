package cursor

import (
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/agent"
	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// SpawnOptions describes everything needed to launch one Cursor Agent
// session inside a tmux pane.
//
// Deliberately a separate type from claude.SpawnOptions rather than a
// shared one: the overlap is the *fields*, which are cheap, while the
// differences are the flag vocabulary, which is the part that matters.
// What the two must not diverge on — the pane scaffolding — is shared
// through agent.Wrapper instead.
type SpawnOptions struct {
	// CLIBinary is the Cursor Agent executable, bare name or path.
	CLIBinary string

	// TaskPrompt is the assembled prompt, passed positionally.
	TaskPrompt string

	// Workspace is the primary worktree, passed as --workspace. Cursor
	// defaults to the process cwd; naming it explicitly means the pane's
	// directory and the agent's workspace can't drift apart.
	Workspace string

	// AddDirs are additional workspace roots (--add-dir), one per extra
	// repo on the track.
	AddDirs []string

	// CWD is the directory tmux opens the pane in.
	CWD string

	// Model is passed as --model. Cursor's ids are its own namespace —
	// "gpt-5.3-codex", "claude-opus-5-thinking-high" — and are not
	// interchangeable with Claude's. Empty omits the flag, leaving
	// Cursor on "auto".
	Model string

	// Mode is passed as --mode for the read-only kinds: "plan" or
	// "ask". Empty omits it.
	Mode string

	// Force allows tool calls without prompting (--force), Cursor's
	// equivalent of Claude's permission-mode auto.
	Force bool

	// ChatID is the id from CreateChat, passed as --resume on every run
	// including the first. Cursor has no --session-id equivalent.
	ChatID string

	// TrackID, SocketDir, BinDir and SentinelPath are passed through to
	// agent.Wrapper, which is where they are documented.
	TrackID      string
	SocketDir    string
	BinDir       string
	SentinelPath string
}

// ShellCommand returns the single shell command tmux should run in the
// pane.
func (o SpawnOptions) ShellCommand() string {
	line := agent.NewLine(o.CLIBinary)
	if o.TaskPrompt != "" {
		line.Arg(o.TaskPrompt)
	}
	// --resume from the first run onwards: the chat was created empty by
	// CreateChat, so there is no separate "start" form.
	line.Set("--resume", o.ChatID)
	if o.Force {
		line.Flag("--force")
	}
	line.SetIf("--mode", o.Mode)
	line.SetIf("--workspace", o.Workspace)
	for _, d := range o.AddDirs {
		line.Set("--add-dir", d)
	}
	line.SetIf("--model", o.Model)

	return agent.Wrapper{
		TrackID:      o.TrackID,
		SocketDir:    o.SocketDir,
		BinDir:       o.BinDir,
		SentinelPath: o.SentinelPath,
	}.Command(line.Build())
}

// BuildOptions assembles SpawnOptions from a Track and Config.
//
// The prompt is composed from the same tracks-level fragments the
// Claude path uses, minus anything that names a Claude mechanism. See
// taskSuffix in this package for what replaces the review gate.
func BuildOptions(cfg config.Config, t state.Track, socketDir, sentinelPath string) (SpawnOptions, error) {
	if len(t.Repos) == 0 && !t.Kind.Worktreeless() {
		return SpawnOptions{}, errors.New("track has no repos")
	}
	// Unlike Claude's locally-generated session uuid, a Cursor chat id
	// comes from a network call that can fail. Omitting --resume would
	// still launch — the agent would open a fresh chat tracks has no id
	// for — producing a track that works once and can never be resumed.
	// Fail instead.
	if t.SessionID == "" {
		return SpawnOptions{}, errors.New("track has no chat ID; run CreateChat before spawning")
	}

	workspace := ""
	addDirs := make([]string, 0, len(t.Repos))
	for i, r := range t.Repos {
		if i == 0 {
			workspace = r.Path
			continue
		}
		addDirs = append(addDirs, r.Path)
	}
	if t.Kind == state.KindDoc {
		if d := t.DocDir(); d != "" {
			if workspace == "" {
				workspace = d
			} else {
				addDirs = append(addDirs, d)
			}
		}
	}

	prompt := strings.TrimRight(t.TaskPrompt, " \t\n\r")
	mode, force := "", true
	switch {
	case t.Kind == state.KindDoc:
		// A doc review writes its report, so it is not read-only. It
		// still prompts rather than running free: --force is dropped so
		// the write is confirmed, matching what the Claude path clamps
		// its permission mode to.
		force = false
		prompt += "\n\n" + docReviewSuffix(t)
	case t.Kind.Worktreeless():
		// plan for KindPlan, ask for KindAsk — Cursor's own read-only
		// modes, which line up with these kinds exactly.
		mode = "plan"
		if t.Kind == state.KindAsk {
			mode = "ask"
		}
		force = false
		if len(t.Repos) > 0 {
			prompt += agent.ReadOnlySuffix
		}
	default:
		prompt += "\n\n" + taskSuffix
		if draft := agent.DraftPRRepos(cfg, t.Repos); len(draft) > 0 {
			prompt += agent.DraftPRSuffix(draft, len(t.Repos))
		}
		if t.Kind == state.KindReview {
			// Without this the candor level the creation form collects
			// is silently discarded on the Cursor path.
			prompt += reviewCandorSuffix(t.CandorLevel())
		}
	}

	// cwd follows the workspace, which for a doc track with repos means
	// the first repo rather than the document's directory. This is the
	// opposite of the Claude path, deliberately: Cursor only loads its
	// rules when run from inside a workspace (see the design doc §11),
	// and the document is reached by absolute path either way.
	cwd := workspace
	if cwd == "" {
		cwd = homeDir()
	}

	model := t.RequestedModel
	if model == "" {
		model = cfg.Cursor.ModelFor(string(t.Kind))
	}

	return SpawnOptions{
		CLIBinary:    cfg.Cursor.Binary,
		TaskPrompt:   prompt,
		Workspace:    workspace,
		AddDirs:      addDirs,
		CWD:          cwd,
		Model:        model,
		Mode:         mode,
		Force:        force,
		ChatID:       t.SessionID,
		TrackID:      t.ID,
		SocketDir:    socketDir,
		SentinelPath: sentinelPath,
	}, nil
}

// BuildResumeOptions assembles SpawnOptions for continuing a finished
// track's chat. No prompt is assembled — the chat picks up where it
// left off.
//
// No --model, matching the Claude path: a resumed session restores what
// it was last using, and forcing the flag would undo a switch the user
// made in the pane.
func BuildResumeOptions(cfg config.Config, t state.Track, socketDir, sentinelPath string) (SpawnOptions, error) {
	if t.SessionID == "" {
		return SpawnOptions{}, errors.New("track has no chat ID; cannot resume")
	}
	opts, err := BuildOptions(cfg, t, socketDir, sentinelPath)
	if err != nil {
		return SpawnOptions{}, err
	}
	opts.TaskPrompt = ""
	opts.Model = ""
	return opts, nil
}
