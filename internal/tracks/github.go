package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	// OpenThreads is how many review threads on pr wait on the gh user:
	// unresolved, with someone else having the last word.
	OpenThreads(ctx context.Context, pr track.PR) (int, error)
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

// threadsQuery reads the last comment of the first 100 review threads,
// and who asks.
const threadsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes { isResolved comments(last: 1) { nodes { author { login } } } }
      }
    }
  }
}`

func (g GH) OpenThreads(ctx context.Context, pr track.PR) (int, error) {
	owner, name, _ := strings.Cut(pr.Repo, "/")
	out, err := g.run(ctx, "", "api", "graphql", "-f", "query="+threadsQuery,
		"-f", "owner="+owner, "-f", "name="+name, "-F", "number="+strconv.Itoa(pr.Number))
	if err != nil {
		return 0, err
	}
	n, err := openThreads(out)
	if err != nil {
		return 0, fmt.Errorf("gh api graphql for %s: %w", pr.URL, err)
	}
	return n, nil
}

// openThreads counts the threads in a threadsQuery answer that are
// unresolved and whose last comment isn't the viewer's.
func openThreads(answer []byte) (int, error) {
	var a struct {
		Data struct {
			Viewer     struct{ Login string }
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool
							Comments   struct {
								Nodes []struct {
									Author *struct{ Login string }
								}
							}
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(answer, &a); err != nil {
		return 0, err
	}
	n := 0
	for _, t := range a.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if t.IsResolved || len(t.Comments.Nodes) == 0 {
			continue
		}
		last := t.Comments.Nodes[len(t.Comments.Nodes)-1].Author
		if last == nil || last.Login != a.Data.Viewer.Login {
			n++
		}
	}
	return n, nil
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
