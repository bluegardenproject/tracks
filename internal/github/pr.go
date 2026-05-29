// Package github wraps the small subset of GitHub interactions
// the tracks dashboard needs. We deliberately don't pull a real
// REST client — every call here can be expressed as one `gh`
// subcommand, and the user already has `gh` authenticated.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PRStatus is the snapshot the dashboard cares about. Fields stay
// strings so they round-trip cleanly through the daemon's
// state.json without bespoke marshaling.
type PRStatus struct {
	State        string // OPEN / CLOSED / MERGED
	Draft        bool
	ReviewState  string // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED / ""
	CommentCount int
}

// ghPRView is the JSON shape `gh pr view <url> --json ...` returns
// for the fields we ask for. Comments and reviews each emit one
// item per entity; we sum them.
type ghPRView struct {
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	Comments       []struct {
		ID string `json:"id"`
	} `json:"comments"`
	Reviews []struct {
		ID string `json:"id"`
	} `json:"reviews"`
}

// Inspect runs `gh pr view <url> --json ...` and returns a
// PRStatus. ctx caps how long we'll wait for `gh` to respond.
//
// Failure modes — all surface as an error:
//   - `gh` not on PATH (caller can downgrade to "skip polling")
//   - user not authenticated for that repo
//   - URL is malformed / PR doesn't exist
func Inspect(ctx context.Context, url string) (PRStatus, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return PRStatus{}, fmt.Errorf("gh not on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", url,
		"--json", "state,isDraft,reviewDecision,comments,reviews")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return PRStatus{}, fmt.Errorf("gh pr view: %w: %s", err, stderr)
	}
	var view ghPRView
	if err := json.Unmarshal(out, &view); err != nil {
		return PRStatus{}, fmt.Errorf("decode gh json: %w", err)
	}
	return PRStatus{
		State:        view.State,
		Draft:        view.IsDraft,
		ReviewState:  view.ReviewDecision,
		CommentCount: len(view.Comments) + len(view.Reviews),
	}, nil
}
