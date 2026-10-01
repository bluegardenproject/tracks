package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

// GitHub asks GitHub where pull requests are.
type GitHub interface {
	// PR is where the pull request at url is.
	PR(ctx context.Context, url string) (track.PRState, error)
	// BranchPRs are the pull requests from branch in the repo checked
	// out at dir, in any state.
	BranchPRs(ctx context.Context, dir, branch string) ([]track.PR, error)
}

// ErrNoGH means gh isn't installed, so no PR is checked.
var ErrNoGH = errors.New("gh isn't installed, so pull requests aren't checked")

// ghTimeout bounds one gh call.
const ghTimeout = 20 * time.Second

// GH asks GitHub with the gh CLI, as the user is logged in to it.
type GH struct {
	Program string // "" is gh on PATH
}

type ghPR struct {
	URL     string `json:"url"`
	State   string `json:"state"`
	IsDraft bool   `json:"isDraft"`
}

func (p ghPR) state() track.PRState {
	switch {
	case p.State == "MERGED":
		return track.PRMerged
	case p.State == "CLOSED":
		return track.PRClosed
	case p.IsDraft:
		return track.PRDraft
	}
	return track.PROpen
}

func (g GH) PR(ctx context.Context, url string) (track.PRState, error) {
	out, err := g.run(ctx, "", "pr", "view", url, "--json", "state,isDraft")
	if err != nil {
		return "", err
	}
	var p ghPR
	if err := json.Unmarshal(out, &p); err != nil {
		return "", fmt.Errorf("gh pr view %s: %w", url, err)
	}
	return p.state(), nil
}

func (g GH) BranchPRs(ctx context.Context, dir, branch string) ([]track.PR, error) {
	out, err := g.run(ctx, dir, "pr", "list", "--head", branch, "--state", "all", "--json", "url,state,isDraft")
	if err != nil {
		return nil, err
	}
	var list []ghPR
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("gh pr list --head %s: %w", branch, err)
	}
	var prs []track.PR
	for _, p := range list {
		if pr, ok := track.ParsePR(p.URL); ok {
			pr.State = p.state()
			prs = append(prs, pr)
		}
	}
	return prs, nil
}

func (g GH) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	program := g.Program
	if program == "" {
		var err error
		if program, err = exec.LookPath("gh"); err != nil {
			return nil, ErrNoGH
		}
	}
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			msg, _, _ = strings.Cut(strings.TrimSpace(string(ee.Stderr)), "\n")
		}
		return nil, fmt.Errorf("gh %s %s: %s", args[0], args[1], msg)
	}
	return out, nil
}
