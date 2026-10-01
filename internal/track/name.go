package track

import (
	"fmt"
	"path/filepath"
	"strings"
)

// labelMaxLen caps a window name built from a prompt, as in v1.
const labelMaxLen = 32

// nameAttempts bounds the -2, -3 ... suffixes, as in v1.
const nameAttempts = 50

// Label is s as a window name: lowercase ASCII letters and digits, every
// other run a single hyphen, cut at a word near labelMaxLen. tmux's
// target separators and spaces never get in. "" when s has nothing
// usable.
func Label(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		switch alnum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'; {
		case alnum:
			if b.Len() >= labelMaxLen {
				return strings.TrimRight(b.String(), "-")
			}
			b.WriteRune(r)
			hyphen = false
		case !hyphen && b.Len() > 0:
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// WindowLabel is what a track's window is called before taken names
// are avoided: from its name, else a document's file name, else its
// prompt.
func WindowLabel(name, document, prompt string) string {
	if strings.TrimSpace(name) == "" && document != "" {
		base := filepath.Base(document)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if l := Label(name); l != "" {
		return l
	}
	return Label(prompt)
}

// WindowName is label, or label-2, label-3 ... when taken says it's
// used. Without a label, or once the suffixes run out, the ID's last
// six characters make it unique.
func WindowName(label, id string, taken func(string) bool) string {
	suffix := id[max(0, len(id)-6):]
	if label == "" {
		return "t-" + suffix
	}
	for n := 1; n <= nameAttempts; n++ {
		name := label
		if n > 1 {
			name = fmt.Sprintf("%s-%d", label, n)
		}
		if !taken(name) {
			return name
		}
	}
	return label + "-" + suffix
}

// Branch is a work track's first branch, which the agent renames:
// tracks/ and the ID's last six characters, as in v1.
func Branch(id string) string { return "tracks/" + id[max(0, len(id)-6):] }
