package agents

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/bluegardenproject/tracks/internal/termtext"
)

// ErrNotFound is returned when an engine's program isn't on PATH.
var ErrNotFound = errors.New("not found")

// Found is an installed engine.
type Found struct {
	Path    string
	Version string // "" when the CLI didn't say
}

const versionTimeout = 5 * time.Second

// Check finds e's program on PATH and asks it for its version.
func Check(ctx context.Context, e Engine) (Found, error) {
	path, err := exec.LookPath(e.Program)
	if err != nil {
		return Found{}, fmt.Errorf("%s isn't on your PATH: %w", e.Program, ErrNotFound)
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return Found{Path: path}, nil
	}
	return Found{Path: path, Version: strings.TrimSpace(clean(version(termtext.Multiline(string(out)))))}, nil
}

// version reads a version from --version output: its first line,
// without a trailing "(Claude Code)"-style name, or a leading "GitHub
// Copilot CLI"-style one and the full stop after it.
func version(out string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	line = strings.TrimSpace(line)
	if i := strings.Index(line, " ("); i > 0 && strings.HasSuffix(line, ")") {
		line = line[:i]
	}
	if fields := strings.Fields(line); len(fields) > 1 {
		if last := strings.TrimSuffix(fields[len(fields)-1], "."); last != "" && unicode.IsDigit(rune(last[0])) {
			return last
		}
	}
	return line
}

// clean drops escape sequences, control and invisible format characters
// from CLI output before it's drawn: escapes would reach the terminal,
// and zero-width spaces throw the columns off.
func clean(s string) string {
	return termtext.SingleLine(s)
}
