package relay

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Outcome is the short status a sender gives its message, "" for none:
// the Relay Station draws it as a badge.
type Outcome string

const (
	Pass       Outcome = "pass"        // a check passed
	Blocked    Outcome = "blocked"     // something stops the work
	Done       Outcome = "done"        // the task is done
	NeedsInput Outcome = "needs-input" // the sender waits on a decision
)

// Outcomes are the outcomes a message may have.
var Outcomes = []Outcome{Pass, Blocked, Done, NeedsInput}

// Refs are what a message is about: PRs by their URLs and files by
// their paths in the repo. Tracks shows them, never follows them.
type Refs struct {
	PRs   []string `json:"prs,omitempty"`
	Files []string `json:"files,omitempty"`
}

// Empty reports whether r refers to nothing.
func (r Refs) Empty() bool { return len(r.PRs) == 0 && len(r.Files) == 0 }

// IsZero is Empty, so a message's JSON leaves out refs to nothing.
func (r Refs) IsZero() bool { return r.Empty() }

// All is r's PRs, then its files.
func (r Refs) All() []string { return slices.Concat(r.PRs, r.Files) }

// The most refs a message may have, and the longest one, in bytes.
const (
	MaxRefs   = 20
	MaxRefLen = 512
)

// Fields are what a sender gives its message besides its body, as it
// typed them: an outcome, and refs not yet resolved.
type Fields struct {
	Outcome Outcome  `json:"outcome,omitempty"`
	Refs    []string `json:"refs,omitempty"`
}

// CheckOutcome says why o isn't an outcome a message may have, nil when
// it is; "" is none.
func CheckOutcome(o Outcome) error {
	if o == "" || slices.Contains(Outcomes, o) {
		return nil
	}
	return fmt.Errorf("%q isn't an outcome: pass, blocked, done or needs-input", o)
}

// Resolve is f's outcome and refs, checked: each ref a PR's URL, #123
// for a PR of repo (owner/name, "" when the track has none to tell it
// by), or a file's path in the repo; at most MaxRefs, and duplicates
// once.
func (f Fields) Resolve(repo string) (Outcome, Refs, error) {
	if err := CheckOutcome(f.Outcome); err != nil {
		return "", Refs{}, err
	}
	if len(f.Refs) > MaxRefs {
		return "", Refs{}, fmt.Errorf("a message has at most %d refs, not %d", MaxRefs, len(f.Refs))
	}
	var refs Refs
	for _, raw := range f.Refs {
		pr, file, err := parseRef(raw, repo)
		switch {
		case err != nil:
			return "", Refs{}, err
		case pr != "" && !slices.Contains(refs.PRs, pr):
			refs.PRs = append(refs.PRs, pr)
		case file != "" && !slices.Contains(refs.Files, file):
			refs.Files = append(refs.Files, file)
		}
	}
	return f.Outcome, refs, nil
}

// CheckRefs says why r can't be a message's refs, nil when it can: what
// Resolve gives, as it gives it.
func CheckRefs(r Refs) error {
	if n := len(r.PRs) + len(r.Files); n > MaxRefs {
		return fmt.Errorf("a message has at most %d refs, not %d", MaxRefs, n)
	}
	for _, pr := range r.PRs {
		if got, _, err := parseRef(pr, ""); err != nil || got != pr {
			return fmt.Errorf("%q isn't a pull request's URL", pr)
		}
	}
	for _, f := range r.Files {
		if _, got, err := parseRef(f, ""); err != nil || got != f {
			return fmt.Errorf("%q isn't a file's path in the repo", f)
		}
	}
	return nil
}

var (
	prNumber = regexp.MustCompile(`^#([1-9][0-9]{0,8})$`)
	prPath   = regexp.MustCompile(`^/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([1-9][0-9]{0,8})$`)
	repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// parseRef is raw as a PR's URL or a file's path, whichever it is.
func parseRef(raw, repo string) (pr, file string, err error) {
	switch {
	case raw == "":
		return "", "", errors.New("a ref is empty")
	case len(raw) > MaxRefLen:
		return "", "", fmt.Errorf("a ref is at most %d bytes: %.40q… is %d", MaxRefLen, raw, len(raw))
	case strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || !unicode.IsPrint(r) || r == '`' }) >= 0:
		return "", "", fmt.Errorf("%q has spaces, control characters or backticks, which no ref has", raw)
	}
	if m := prNumber.FindStringSubmatch(raw); m != nil {
		if !repoName.MatchString(repo) {
			return "", "", fmt.Errorf("%s needs the track's repo, and Tracks knows none from its PRs: give the PR's URL", raw)
		}
		return "https://github.com/" + repo + "/pull/" + m[1], "", nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || !prPath.MatchString(strings.TrimSuffix(u.Path, "/")) {
			return "", "", fmt.Errorf("%q isn't a pull request's URL, such as https://github.com/owner/repo/pull/123", raw)
		}
		return "https://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/"), "", nil
	}
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "-") || strings.Contains(raw, `\`) ||
		path.Clean(raw) != raw || raw == "." || raw == ".." || strings.HasPrefix(raw, "../") {
		return "", "", fmt.Errorf("%q isn't a PR or a file's path in the repo, such as internal/relay/relay.go", raw)
	}
	return "", raw, nil
}

// PRLabel is a PR's URL as a ref shows it: owner/name#123.
func PRLabel(prURL string) string {
	u, err := url.Parse(prURL)
	if err != nil {
		return prURL
	}
	m := prPath.FindStringSubmatch(u.Path)
	if m == nil {
		return prURL
	}
	return m[1] + "/" + m[2] + "#" + m[3]
}
