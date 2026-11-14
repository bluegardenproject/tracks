// Package commentstyle learns how the user writes PR comments: it
// collects their recent comments from GitHub, leaves out those Tracks
// wrote under their name, and has an engine distil them into a guide
// the agents follow when they reply. Each step reports to a log.
package commentstyle

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Marker ends every comment an agent posts through Tracks, hidden by
// GitHub's Markdown, so learning can tell them from the user's own.
const Marker = "<!-- tracks -->"

// Window is how far back comments are collected.
const Window = 60 * 24 * time.Hour

// FileName is the guide's name in the config directory.
const FileName = "comment-style.md"

// Path is where the guide is kept.
func Path(configDir string) string { return filepath.Join(configDir, FileName) }

// Comment is one of the user's comments.
type Comment struct {
	PR   string // owner/name#number
	Kind string // "review" for a review-thread comment, "pr" for one on the PR
	Body string
	At   time.Time
}

// Status is what the Settings section shows of the guide.
type Status struct {
	Learned  bool
	At       time.Time
	Comments int
}

// header opens the guide with when it was learned and from how many
// comments.
func header(at time.Time, comments int) string {
	return fmt.Sprintf("<!-- tracks comment style: learned %s from %d comments -->", at.UTC().Format(time.RFC3339), comments)
}

var headerRE = regexp.MustCompile(`^<!-- tracks comment style: learned (\S+) from (\d+) comments -->`)

// ReadStatus reads the guide at path's header; a missing guide isn't
// learned, and one without the header was written by hand.
func ReadStatus(path string) (Status, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return Status{}, nil
	} else if err != nil {
		return Status{}, err
	}
	defer f.Close()
	first, _ := bufio.NewReader(f).ReadString('\n')
	s := Status{Learned: true}
	if m := headerRE.FindStringSubmatch(first); m != nil {
		s.At, _ = time.Parse(time.RFC3339, m[1])
		s.Comments, _ = strconv.Atoi(m[2])
	}
	return s, nil
}

// Save writes guide to path under its header.
func Save(path, guide string, at time.Time, comments int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Written aside and renamed, so an agent never reads half a guide.
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(header(at, comments)+"\n\n"+strings.TrimSpace(guide)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
