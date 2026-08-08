// Package dlog is the tracks daemon's log sink.
//
// The daemon is launched with `tmux run-shell -b`, which discards the
// child's stderr — so anything the daemon prints there is lost, which is
// exactly the information a user needs when a track ends up in an
// unexpected state. Init points the sink at a file under the state dir;
// until it is called (tests, and the CLI's own commands) output goes to
// stderr alone, so nothing has to be initialised to be loggable.
//
// This is deliberately a package-level logger rather than a value passed
// down: the alternative is threading a logger through every daemon
// subsystem for what is, in a single-process daemon, one file.
package dlog

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// FileName is the log file's name within the directory passed to Init.
const FileName = "daemon.log"

var (
	mu     sync.Mutex
	sink   *os.File
	path   string
	logger = log.New(os.Stderr, "", log.LstdFlags)
)

// Init opens <dir>/daemon.log for appending and sends every subsequent
// Printf there as well as to stderr, returning the file's path. Appending
// (rather than truncating) keeps the previous daemon's final lines around
// — a crash is usually diagnosed from the run before the current one.
//
// Calling Init again closes the previous file and switches to the new one.
func Init(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, FileName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()
	if sink != nil {
		_ = sink.Close()
	}
	sink = f
	path = p
	logger.SetOutput(io.MultiWriter(os.Stderr, f))
	return p, nil
}

// Printf writes one timestamped line to the sink. Safe for concurrent use
// (log.Logger serialises writes internally), and safe before Init.
func Printf(format string, v ...any) {
	logger.Printf(format, v...)
}

// Path is the log file currently being written to, or "" when Init hasn't
// run and output is going to stderr only.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// Close releases the log file and reverts the sink to stderr.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if sink == nil {
		return
	}
	logger.SetOutput(os.Stderr)
	_ = sink.Close()
	sink = nil
	path = ""
}
