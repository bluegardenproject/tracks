package review

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Packet is what a reviewer is given beside its instructions: the
// change, and what's around it. The caller gathers it, from git, in
// the agent's sandbox.
type Packet struct {
	// Base is the ref the change is against.
	Base string `json:"base"`
	// Brief is what the user or the track's agent asks of this review.
	Brief string `json:"brief,omitempty"`
	// Commits are the branch's commit messages: what the change is for.
	Commits string `json:"commits,omitempty"`
	// Files are every changed file, Diffs those whose diff is given and
	// Left those over the limit, left out whole.
	Files []File     `json:"files"`
	Diffs []FileDiff `json:"diffs"`
	Left  []string   `json:"left,omitempty"`
	// MoreFiles counts the changed files over the list's limit.
	MoreFiles int `json:"more_files,omitempty"`
}

// File is a changed file and its added and removed lines; a binary
// file has none.
type File struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary,omitempty"`
}

// FileDiff is one file's diff, committed on the branch or not yet.
type FileDiff struct {
	Path        string `json:"path"`
	Text        string `json:"text"`
	Uncommitted bool   `json:"uncommitted,omitempty"`
}

// The packet's limits. Diffs over maxDiffBytes leave whole files out,
// the largest first, so no file is cut in the middle; the commits and
// the brief are cut with a note, and the changed files listed up to
// maxFiles. Together, even escaped as JSON, they stay well within the
// daemon's request limit.
const (
	maxDiffBytes    = 1_000_000
	maxCommitsBytes = 64_000
	maxBriefBytes   = 64_000
	maxFiles        = 3_000
)

// Empty reports whether p has no change to review.
func (p Packet) Empty() bool { return len(p.Files) == 0 && len(p.Diffs) == 0 }

// Fit keeps p within the limits. A packet fitted before is kept as it
// is, so the caller and the daemon can both fit it.
func (p Packet) Fit() Packet {
	var left []string
	p.Diffs, left = fitDiffs(p.Diffs, maxDiffBytes)
	seen := map[string]bool{}
	for _, l := range p.Left {
		seen[l] = true
	}
	for _, l := range left {
		if !seen[l] {
			seen[l] = true
			p.Left = append(p.Left, l)
		}
	}
	if len(p.Left) > maxFiles {
		p.Left = p.Left[:maxFiles]
	}
	if len(p.Files) > maxFiles {
		p.MoreFiles += len(p.Files) - maxFiles
		p.Files = p.Files[:maxFiles]
	}
	p.Commits = cut(p.Commits, maxCommitsBytes)
	p.Brief = cut(p.Brief, maxBriefBytes)
	return p
}

// fitDiffs leaves out whole diffs, the largest first, until the rest
// fit in limit, and keeps the rest in their order. left names each
// file once, in its order.
func fitDiffs(diffs []FileDiff, limit int) (kept []FileDiff, left []string) {
	total := 0
	for _, d := range diffs {
		total += len(d.Text)
	}
	order := make([]int, len(diffs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(len(diffs[b].Text), len(diffs[a].Text)) })
	drop := map[int]bool{}
	for _, i := range order {
		if total <= limit {
			break
		}
		drop[i] = true
		total -= len(diffs[i].Text)
	}
	seen := map[string]bool{}
	for i, d := range diffs {
		if !drop[i] {
			kept = append(kept, d)
		} else if !seen[d.Path] {
			seen[d.Path] = true
			left = append(left, d.Path)
		}
	}
	return kept, left
}

// cut cuts text over limit bytes, with a note. A text cut before is
// left as it is.
func cut(text string, limit int) string {
	if len(text) <= limit || cutNote.MatchString(text) && len(text) <= limit+maxNote {
		return text
	}
	return strings.ToValidUTF8(text[:limit], "") + "\n\n[cut at " + strconv.Itoa(limit) + " bytes of " + strconv.Itoa(len(text)) + "]"
}

// maxNote bounds the length of cut's note.
const maxNote = 64

// cutNote matches the note cut ends a text with.
var cutNote = regexp.MustCompile(`\n\n\[cut at \d+ bytes of \d+\]\z`)

// SplitDiff splits a unified git diff into its files.
func SplitDiff(diff string, uncommitted bool) []FileDiff {
	var out []FileDiff
	for _, part := range strings.SplitAfter(diff, "\n") {
		if strings.HasPrefix(part, "diff --git ") || len(out) == 0 {
			if strings.TrimSpace(part) == "" && len(out) == 0 {
				continue
			}
			out = append(out, FileDiff{Path: diffPath(part), Uncommitted: uncommitted})
		}
		out[len(out)-1].Text += part
	}
	return out
}

// diffPath is the file a "diff --git a/<old> b/<new>" line is about:
// its new path. With spaces in a path, the split that gives the same
// path on both sides wins, as on every file that isn't renamed.
func diffPath(line string) string {
	rest, ok := strings.CutPrefix(strings.TrimRight(line, "\n"), "diff --git ")
	if !ok {
		return ""
	}
	if strings.HasSuffix(rest, `"`) {
		// git quotes a path with special characters, C-style.
		for i := len(rest) - 2; i >= 0; i-- {
			if rest[i] != '"' || i > 0 && rest[i-1] != ' ' {
				continue
			}
			if q, err := strconv.Unquote(rest[i:]); err == nil {
				return strings.TrimPrefix(q, "b/")
			}
		}
	}
	last := ""
	for i := 0; i < len(rest); i++ {
		if !strings.HasPrefix(rest[i:], " b/") {
			continue
		}
		old, new := strings.TrimPrefix(rest[:i], "a/"), rest[i+3:]
		if old == new {
			return new
		}
		last = new
	}
	return last
}

// ParseNumstat reads `git diff --numstat -z --no-renames` output into
// files.
func ParseNumstat(out string) []File {
	var files []File
	for _, line := range strings.Split(out, "\x00") {
		line = strings.TrimPrefix(line, "\n")
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		f := File{Path: parts[2]}
		if parts[0] == "-" && parts[1] == "-" {
			f.Binary = true
		} else {
			f.Added, _ = strconv.Atoi(parts[0])
			f.Removed, _ = strconv.Atoi(parts[1])
		}
		files = append(files, f)
	}
	return files
}

// MergeFiles adds the files of b to a, summing a file's lines that's in
// both, such as one changed in a commit and again since.
func MergeFiles(a, b []File) []File {
	out := slices.Clone(a)
	at := map[string]int{}
	for i, f := range out {
		at[f.Path] = i
	}
	for _, f := range b {
		i, ok := at[f.Path]
		if !ok {
			at[f.Path] = len(out)
			out = append(out, f)
			continue
		}
		out[i].Added += f.Added
		out[i].Removed += f.Removed
		out[i].Binary = out[i].Binary || f.Binary
	}
	return out
}
