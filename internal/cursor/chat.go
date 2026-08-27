// Package cursor wraps how `tracks` invokes the Cursor Agent CLI as a
// long-running interactive agent inside a tmux window.
//
// It mirrors internal/claude: same lifecycle, same tmux pane, same
// sentinel. What differs is the flag vocabulary, and that Cursor's
// session id has to be created before the first run rather than
// generated locally.
package cursor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// chatIDTimeout bounds `agent create-chat`. It is a single round trip
// to Cursor's API and returns in well under a second in practice.
const chatIDTimeout = 30 * time.Second

// chatIDRE matches the chat id Cursor returns. Anchored, and applied
// per line rather than to the last line alone: the CLI prefixes
// warnings today and could append a hint tomorrow, and a bare length
// check would accept a 36-character prose line as an id and store it
// as the track's SessionID forever.
var chatIDRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// CreateChat asks the Cursor CLI for a new empty chat and returns its
// id, which becomes the track's SessionID.
//
// Claude generates its session uuid locally and passes it in with
// --session-id; Cursor has no equivalent, so the id must be obtained
// first and then passed to every run as --resume, including the first.
//
// Two details are load-bearing, both learned the hard way:
//
//   - Stdin is left nil, so the child gets /dev/null. An inherited
//     terminal stdin makes the real CLI block indefinitely printing
//     nothing — inside provisioning that is a hang with no error,
//     strictly worse than a failure.
//   - The deadline is belt to that brace, and WaitDelay is belt to the
//     deadline: killing the process does not close a pipe a surviving
//     grandchild still holds, and Output() blocks until every writer
//     is gone. `agent` is a Node CLI, so descendants are plausible.
func CreateChat(ctx context.Context, binary string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		return "", errors.New("cursor binary is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, chatIDTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "create-chat")
	cmd.Stdin = nil // /dev/null — an inherited terminal stdin hangs forever
	// Bound the wait even if a descendant keeps the output pipe open
	// after the context kills the direct child.
	cmd.WaitDelay = 5 * time.Second

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// Don't name a duration that may not be the one that fired:
			// the caller's context can carry a shorter deadline.
			return "", fmt.Errorf("%s create-chat timed out", binary)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s create-chat: %s", binary, firstLine(ee.Stderr))
		}
		return "", fmt.Errorf("%s create-chat: %w", binary, err)
	}

	id := findChatID(string(out))
	if id == "" {
		return "", fmt.Errorf("%s create-chat printed no chat id: %q", binary, truncate(string(out)))
	}
	return id, nil
}

// findChatID returns the first line that is a chat id, or "". Scanning
// every line rather than trusting position: the CLI prints unrelated
// warnings before the id today (certificate noise on macOS) and is
// free to print something after it tomorrow.
func findChatID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if l := strings.TrimSpace(line); chatIDRE.MatchString(l) {
			return l
		}
	}
	return ""
}

// truncate bounds CLI output quoted into an error.
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
