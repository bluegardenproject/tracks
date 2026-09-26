package repos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bluegardenproject/tracks/internal/v2/store"
)

// maxName is the longest name, in characters.
const maxName = 64

// validName reports whether name, already trimmed, can be shown as a
// repo's name. Folders get a simplified version when a track starts,
// so any printable text goes.
func validName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > maxName || !utf8.ValidString(name) {
		return false
	}
	return !strings.ContainsFunc(name, unicode.IsControl)
}

// DefaultBase is the base branch of a repo saved without one.
const DefaultBase = "main"

// Suggest reads the checkout at path and proposes a repo for it: its
// top folder, a name from that folder, the branch origin/HEAD points to
// and origin's address.
func (s Service) Suggest(ctx context.Context, path string) (Entry, error) {
	top, err := s.checkout(ctx, path)
	if err != nil {
		return Entry{}, err
	}
	base, _ := s.Git.DefaultBranch(ctx, top)
	return Entry{
		Repo:   store.Repo{Name: nameFrom(filepath.Base(top)), Path: top, BaseBranch: base},
		Remote: s.remote(ctx, top),
	}, nil
}

// check normalizes r's fields and reports the first problem.
func (s Service) check(ctx context.Context, r store.Repo) (store.Repo, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.BaseBranch = strings.TrimSpace(r.BaseBranch)
	if !validName(r.Name) {
		return r, &FieldError{"name", fmt.Sprintf("Enter a name of up to %d characters.", maxName)}
	}
	top, err := s.checkout(ctx, r.Path)
	if err != nil {
		return r, err
	}
	r.Path = top
	if r.BaseBranch == "" {
		r.BaseBranch = DefaultBase
	}
	if ok, err := s.Git.BranchExists(ctx, top, r.BaseBranch); err != nil {
		return r, err
	} else if !ok {
		return r, &FieldError{"base", "No branch " + r.BaseBranch + " here or on origin."}
	}
	return r, nil
}

// checkout resolves path to the top folder of a primary git checkout.
func (s Service) checkout(ctx context.Context, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", &FieldError{"path", "Enter the path to the repo's checkout."}
	}
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/') {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home + rest
	}
	if !filepath.IsAbs(path) {
		return "", &FieldError{"path", "Use an absolute path, or one starting with ~/."}
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", &FieldError{"path", "No folder at this path."}
	}
	co, err := s.Git.Checkout(ctx, resolved)
	if err != nil {
		return "", &FieldError{"path", "This folder isn't a git checkout."}
	}
	if co.Worktree {
		return "", &FieldError{"path", "This is a worktree; add the main checkout instead."}
	}
	return co.Top, nil
}

// nameFrom turns a folder name into a valid repo name.
func nameFrom(folder string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(folder, ""))
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxName {
		name = strings.TrimSpace(string(r[:maxName]))
	}
	return name
}
