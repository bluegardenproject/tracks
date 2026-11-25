package tracks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/track"
)

// GitHub asks GitHub where pull requests are.
type GitHub interface {
	// Poll asks GitHub about the PRs from q's branches and q's PRs, in
	// as few calls as it can. It answers what it could read, and says
	// what it couldn't.
	Poll(ctx context.Context, q Query) (Answer, error)
	// RepoOf is the GitHub repo, owner/name, of the checkout at dir.
	RepoOf(ctx context.Context, dir string) (string, error)
	// ReviewPR is what a review of pr is given from GitHub, the commit
	// its head is at, and notes on what else couldn't be fetched.
	ReviewPR(ctx context.Context, pr track.PR) (review.PR, string, []string, error)
	// PRTitle is pr's title.
	PRTitle(ctx context.Context, pr track.PR) (string, error)
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
	URL       string
	State     string
	IsDraft   bool
	Base      string
	Head      string
	Mergeable string
	Checks    []ghCheck
}

// ghCheck is a check run (Status, Conclusion) or a commit status
// (Context, State).
type ghCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Context    string `json:"context"`
	State      string `json:"state"`
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

func (p ghPR) pr() (track.PR, bool) {
	pr, ok := track.ParsePR(p.URL)
	if !ok {
		return track.PR{}, false
	}
	pr.State = p.state()
	pr.Health = track.Health{Base: p.Base, Head: p.Head, Mergeable: mergeable(p.Mergeable)}
	pr.Health.Checks, pr.Health.Failing = rollup(p.Checks)
	return pr, true
}

func mergeable(m string) track.Mergeable {
	switch m {
	case "MERGEABLE":
		return track.MergeClean
	case "CONFLICTING":
		return track.MergeConflicts
	}
	return track.MergeUnknown
}

// rollup is where checks are, and the names of those that failed. A
// cancelled run doesn't fail: a newer push often cancels it.
func rollup(checks []ghCheck) (track.Checks, []string) {
	var failing []string
	pending := false
	for _, c := range checks {
		name := c.Name
		if name == "" {
			name = c.Context
		}
		switch {
		case c.Conclusion == "FAILURE" || c.Conclusion == "TIMED_OUT" || c.Conclusion == "STARTUP_FAILURE" ||
			c.Conclusion == "ACTION_REQUIRED" || c.State == "FAILURE" || c.State == "ERROR":
			failing = append(failing, name)
		case c.Status != "" && c.Status != "COMPLETED", c.State == "PENDING" || c.State == "EXPECTED":
			pending = true
		}
	}
	switch {
	case len(failing) > 0:
		return track.ChecksFailing, failing
	case pending:
		return track.ChecksPending, nil
	case len(checks) > 0:
		return track.ChecksPassing, nil
	}
	return track.ChecksNone, nil
}

func (g GH) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return g.runTail(ctx, dir, 0, args...)
}

// runTail runs gh and keeps the last limit bytes it prints, all of it
// for 0.
func (g GH) runTail(ctx context.Context, dir string, limit int, args ...string) ([]byte, error) {
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
	out := &tail{limit: limit}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	if err := cmd.Run(); err != nil {
		msg := err.Error()
		if stderr.Len() > 0 {
			msg, _, _ = strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		}
		// What it printed comes too: GraphQL answers what it could
		// alongside an error for the rest.
		return out.buf, fmt.Errorf("gh %s %s: %s", args[0], args[1], msg)
	}
	return out.buf, nil
}

// tail keeps the last limit bytes written to it, all of them for 0.
type tail struct {
	limit int
	buf   []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if t.limit > 0 && len(t.buf) > 2*t.limit {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.limit:]...)
	}
	return len(p), nil
}
