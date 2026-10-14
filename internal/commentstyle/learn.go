package commentstyle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "embed"
)

// Line is one line of the log: a request, its answer, a note, a failure
// or the end.
type Line struct {
	Kind rune
	Text string
}

// The kinds of Line, drawn as their mark.
const (
	Request = '→'
	Answer  = '←'
	Note    = '·'
	Fail    = '✗'
	Done    = '✓'
)

// String is l as it travels: its mark, a space, its text.
func (l Line) String() string { return string(l.Kind) + " " + l.Text }

// ParseLine reads a Line back from String; text without a known mark is
// a Note.
func ParseLine(s string) Line {
	for _, k := range []rune{Request, Answer, Note, Fail, Done} {
		if rest, ok := strings.CutPrefix(s, string(k)+" "); ok {
			return Line{k, rest}
		}
	}
	return Line{Note, s}
}

// Engine is the agent CLI that writes the guide.
type Engine struct {
	ID      string // "claude" or "cursor"
	Name    string // as shown
	Program string // its path
	Model   string // "" for its own default
}

// generateTimeout bounds the engine's run.
const generateTimeout = 5 * time.Minute

//go:embed prompt.md
var promptText string

// Prompt is what the engine is asked, with comments appended.
func Prompt(comments []Comment) string {
	var b strings.Builder
	b.WriteString(promptText)
	for _, c := range comments {
		fmt.Fprintf(&b, "\n--- %s (%s comment, %s)\n%s\n", c.PR, c.Kind, c.At.Format("2006-01-02"), c.Body)
	}
	return b.String()
}

// generate has e write the guide from prompt, on stdin, and returns its
// answer.
func generate(ctx context.Context, e Engine, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	// An empty folder, so the engine reads no project's instructions,
	// hooks or files.
	dir, err := os.MkdirTemp("", "tracks-style-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	args := []string{"-p"}
	if e.ID == "cursor" {
		args = append(args, "--workspace", dir)
	}
	if e.Model != "" {
		args = append(args, "--model", e.Model)
	}
	cmd := exec.CommandContext(ctx, e.Program, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("%s didn't answer within %s", e.Name, generateTimeout)
	}
	if err != nil {
		if msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); msg != "" {
			return "", fmt.Errorf("%s failed: %s", e.Name, msg)
		}
		return "", fmt.Errorf("%s failed: %w", e.Name, err)
	}
	guide := strings.TrimSpace(string(out))
	if guide == "" {
		return "", errors.New(e.Name + " answered with nothing")
	}
	return guide, nil
}

// Learner learns the user's comment style.
type Learner struct {
	API    API
	Engine Engine
	Now    func() time.Time // nil is the real clock
}

// Learn collects the user's comments, has the engine write the guide
// and saves it at path, telling log each step.
func (l Learner) Learn(ctx context.Context, path string, log func(Line)) (Status, error) {
	now := time.Now()
	if l.Now != nil {
		now = l.Now()
	}
	user, err := login(ctx, l.API, log)
	if err != nil {
		return Status{}, err
	}
	comments, err := collect(ctx, l.API, user, now.Add(-Window), log)
	if err != nil {
		return Status{}, err
	}
	if len(comments) == 0 {
		return Status{}, errors.New("you wrote no PR comments in the last 60 days, so there's nothing to learn from")
	}
	prompt := Prompt(comments)
	model := l.Engine.Model
	if model == "" {
		model = "its default model"
	}
	log(Line{Request, fmt.Sprintf("%s -p, %s, %d KB of comments", l.Engine.Name, model, len(prompt)/1024+1)})
	guide, err := generate(ctx, l.Engine, prompt)
	if err != nil {
		return Status{}, err
	}
	log(Line{Answer, fmt.Sprintf("a guide of %d lines", strings.Count(guide, "\n")+1)})
	if err := Save(path, guide, now, len(comments)); err != nil {
		return Status{}, err
	}
	log(Line{Done, "saved " + path})
	return Status{Learned: true, At: now, Comments: len(comments)}, nil
}
