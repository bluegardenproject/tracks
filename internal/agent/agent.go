// Package agent holds what every agent CLI tracks can launch has in
// common: the shell scaffolding a track's tmux pane runs, and the
// prompt fragments that describe tracks itself rather than any
// particular assistant.
//
// The split is deliberate and narrow. A provider package
// (internal/claude, internal/cursor) owns its own flags, its own
// permission vocabulary, and any instruction naming a mechanism only
// that harness has. Everything here is either shell mechanics that
// must be byte-identical between providers, or a statement about how
// tracks works that stays true whoever is reading it.
//
// Notably NOT here: the review-candor fragment, which tells the model
// to pass a line to a review *subagent*, and Claude's permission-mode
// clamping. Both read as generic and are not.
package agent

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/state"
)

// Wrapper is the shell scaffolding tracks puts around an agent command
// inside a tmux pane: the environment every track exports, the
// sentinel that tells the supervisor the agent exited, and the login
// shell the pane falls back to so it stays usable afterwards.
//
// Shared rather than duplicated per provider because a difference here
// is not a wording difference — it is a track that never reports
// finishing, or a helper script that can't find its daemon.
type Wrapper struct {
	// TrackID is exported as TRACKS_ID so in-worktree helper scripts
	// can identify which track is calling. Every process in the pane
	// inherits it, whichever agent binary runs.
	TrackID string

	// SocketDir is exported as TRACKS_SOCKET_DIR so those same scripts
	// can find the daemon.
	SocketDir string

	// BinDir, when set, is prepended to PATH so a bare `tracks`
	// resolves even when the binary isn't on the login-shell PATH.
	BinDir string

	// SentinelPath is touched the instant the agent exits, so the
	// supervisor can finalize the track without depending on pid death.
	// Empty means no sentinel handling.
	SentinelPath string
}

// Command wraps an already-assembled agent command line — flags
// quoted, joined, provider's business — in that scaffolding, and
// returns the single string tmux should run in the new pane.
//
// tmux's `new-window <command>` hands its argument to /bin/sh -c, so
// this must produce a string rather than an argv.
func (w Wrapper) Command(agentLine string) string {
	inner := agentLine
	if w.SentinelPath != "" {
		inner += "\ntouch " + shellx.Quote(w.SentinelPath)
	}
	// The pane stays alive as a login shell once the agent exits, so
	// the user can keep working in the worktree.
	inner += "\nexec ${SHELL:-bash} -l"

	env := "TRACKS_ID=" + shellx.Quote(w.TrackID) +
		" TRACKS_SOCKET_DIR=" + shellx.Quote(w.SocketDir)
	if w.BinDir != "" {
		// $PATH is expanded by the outer shell that runs this line.
		env += " PATH=" + shellx.Quote(w.BinDir) + `:"$PATH"`
	}

	// sh, not bash, for the outer wrapper: /bin/sh is the only shell
	// tmux relies on. The user's $SHELL is invoked only at the fallback
	// step above.
	return env + " sh -c " + shellx.Quote(inner)
}

// ReadOnlySuffix is appended for worktree-less (ask/plan) tracks. They
// point at the user's PRIMARY checkout — the one their editor watches
// — so the prompt makes the read-only contract explicit as a second
// line of defence behind whatever read-only mode the provider offers.
// For Claude that mode is a default rather than a hard sandbox;
// whether Cursor's --mode plan behaves the same has not been
// established, so this fragment carries the contract on its own.
const ReadOnlySuffix = "" +
	"\n\n**This is a read-only track.** You are pointed at the user's " +
	"primary checkout — the working copy their editor uses — NOT a " +
	"throwaway worktree. Do not modify any files, create branches, or " +
	"run mutating commands; investigate and answer (or produce a plan) " +
	"only. When the user is ready to implement, the track can be " +
	"promoted to its own worktree with `tracks promote <id>`."

// DraftPRRepos returns the names of the track's repos configured to
// open pull requests as drafts by default.
func DraftPRRepos(cfg config.Config, repos []state.TrackRepo) []string {
	var out []string
	for _, r := range repos {
		if cr, ok := cfg.RepoByName(r.Name); ok && cr.DraftPRs {
			out = append(out, r.Name)
		}
	}
	return out
}

// DraftPRSuffix builds the prompt fragment instructing the agent to
// open PRs as drafts. When every repo on the track wants drafts (the
// common single-repo case) it stays generic; otherwise it names the
// repos so a mixed-repo track only drafts the ones that opted in.
func DraftPRSuffix(draftRepos []string, totalRepos int) string {
	if len(draftRepos) == totalRepos {
		return "\n\nWhen you open a pull request, open it as a **draft** " +
			"(`gh pr create --draft`) unless the user asks otherwise."
	}
	return "\n\nWhen you open a pull request for any of these repos, open it " +
		"as a **draft** (`gh pr create --draft`) unless the user asks " +
		"otherwise: " + strings.Join(draftRepos, ", ") + "."
}
