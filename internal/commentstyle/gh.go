package commentstyle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ghTimeout bounds one request.
const ghTimeout = 30 * time.Second

// GH is the API through the gh CLI, as the user is logged in to it.
type GH struct {
	Program string // "" is gh on PATH
}

func (g GH) Get(ctx context.Context, path string, query ...string) ([]byte, error) {
	program := g.Program
	if program == "" {
		var err error
		if program, err = exec.LookPath("gh"); err != nil {
			return nil, errors.New("gh isn't installed: Tracks reads your comments with it")
		}
	}
	args := []string{"api", "-X", "GET", path}
	for _, q := range query {
		args = append(args, "-f", q)
	}
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			msg, _, _ = strings.Cut(strings.TrimSpace(string(ee.Stderr)), "\n")
		}
		return nil, fmt.Errorf("gh api %s: %s", path, msg)
	}
	return out, nil
}
