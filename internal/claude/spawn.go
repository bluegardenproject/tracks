// Package claude wraps how `tracks` invokes the `claude` CLI as a
// long-running agent, and how it interprets the stream-json log
// Claude emits.
//
// Spawning is intentionally thin: we exec the binary with the right
// flags, redirect stdout/stderr to a log file, and return the
// running process. The daemon owns supervision and state
// transitions; this package just knows about Claude's own surface.
package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// SpawnOptions describes everything `tracks` needs to launch one
// Claude session.
type SpawnOptions struct {
	// CLIBinary is the claude executable. Either a bare name (PATH
	// lookup) or an absolute path.
	CLIBinary string

	// PermissionMode is passed as --permission-mode.
	PermissionMode string

	// TaskPrompt is the assembled prompt (task + DefaultPromptSuffix).
	TaskPrompt string

	// AddDirs are the absolute paths passed as --add-dir for each
	// worktree the track owns.
	AddDirs []string

	// CWD is the directory claude is launched from. Conventionally
	// the first AddDir, so paths in the assistant's output are
	// relative to the primary repo.
	CWD string

	// LogPath is where stdout (and stderr) get appended. Created if
	// missing.
	LogPath string

	// TrackID is exported as TRACKS_ID in the child's env so any
	// in-worktree helper script (e.g. tracks-add-repo) can identify
	// which track is calling.
	TrackID string

	// SocketDir is exported as TRACKS_SOCKET_DIR so the same helper
	// scripts can find the daemon.
	SocketDir string
}

// BuildOptions assembles SpawnOptions from a Track and Config.
// Returns an error when the configuration is incomplete (e.g. no
// worktrees on the track).
func BuildOptions(cfg config.Config, t state.Track, socketDir string) (SpawnOptions, error) {
	if len(t.Repos) == 0 {
		return SpawnOptions{}, errors.New("track has no repos")
	}
	addDirs := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		addDirs = append(addDirs, r.Path)
	}
	prompt := t.TaskPrompt
	if cfg.Claude.DefaultPromptSuffix != "" {
		prompt += cfg.Claude.DefaultPromptSuffix
	}
	return SpawnOptions{
		CLIBinary:      cfg.Claude.Binary,
		PermissionMode: cfg.Claude.PermissionMode,
		TaskPrompt:     prompt,
		AddDirs:        addDirs,
		CWD:            t.Repos[0].Path,
		LogPath:        t.LogPath,
		TrackID:        t.ID,
		SocketDir:      socketDir,
	}, nil
}

// Args returns the argv (minus argv[0]) `tracks` invokes claude with.
// Exported for tests and for the verbose logging path.
func (o SpawnOptions) Args() []string {
	args := []string{
		"-p", o.TaskPrompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if o.PermissionMode != "" {
		args = append(args, "--permission-mode", o.PermissionMode)
	}
	for _, d := range o.AddDirs {
		args = append(args, "--add-dir", d)
	}
	return args
}

// Spawn starts the Claude process and returns it. The caller is
// responsible for Wait()ing on it (typically in a supervisor
// goroutine).
//
// The log file is opened in append mode so a restarted daemon can
// preserve history. Stdout and stderr are merged — Claude's
// stream-json format is on stdout, but any setup errors come on
// stderr, and we want both in the same artifact for post-mortems.
func Spawn(opts SpawnOptions) (*exec.Cmd, error) {
	if opts.CLIBinary == "" {
		return nil, errors.New("CLIBinary is required")
	}
	if opts.LogPath == "" {
		return nil, errors.New("LogPath is required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	logFile, err := os.OpenFile(opts.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", opts.LogPath, err)
	}

	cmd := exec.Command(opts.CLIBinary, opts.Args()...)
	if opts.CWD != "" {
		cmd.Dir = opts.CWD
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Detach stdin so claude doesn't try to read from our terminal.
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		"TRACKS_ID="+opts.TrackID,
		"TRACKS_SOCKET_DIR="+opts.SocketDir,
	)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start claude: %w", err)
	}
	// We deliberately don't close logFile — the child has it. When
	// the child exits, its descriptor closes and our copy can be GC'd.
	return cmd, nil
}

// CommandPreview returns a shell-quoted preview of the argv, useful
// for the --verbose flag and for the daemon log.
func (o SpawnOptions) CommandPreview() string {
	parts := []string{o.CLIBinary}
	for _, a := range o.Args() {
		if strings.ContainsAny(a, " \t\n\"'`$") {
			parts = append(parts, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
