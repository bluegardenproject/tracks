package review

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// CodeownersPaths are where GitHub looks for a repo's CODEOWNERS, in
// order: the first found is the one.
var CodeownersPaths = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// Codeowners is a CODEOWNERS file's rules, in the file's order.
type Codeowners []ownerRule

type ownerRule struct {
	match  *regexp.Regexp
	owners []string
}

// ParseCodeowners reads a CODEOWNERS file. Lines it can't read, such as
// a pattern GitHub doesn't support, are skipped, as GitHub skips them.
func ParseCodeowners(text string) Codeowners {
	var rules Codeowners
	for _, line := range strings.Split(text, "\n") {
		fields := ownerFields(line)
		if len(fields) == 0 {
			continue
		}
		re, ok := ownerPattern(fields[0])
		if !ok {
			continue
		}
		rules = append(rules, ownerRule{match: re, owners: fields[1:]})
	}
	return rules
}

// ownerFields are a line's pattern and owners, without its comment. A
// backslash escapes a space or a # in the pattern, and keeps any other
// character literal; a line starting with \# is skipped, since GitHub
// doesn't take that escape.
func ownerFields(line string) []string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, `\#`) {
		return nil
	}
	var fields []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			if r != ' ' && r != '#' {
				cur.WriteRune('\\')
			}
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '#':
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
			}
			return fields
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// ownerPattern is a CODEOWNERS pattern as a match on a path from the
// repo's root, with GitHub's rules: gitignore's, where a name also
// covers everything in a folder of that name, but no negation or
// character ranges, and a trailing /* matches only the files directly
// in its folder.
func ownerPattern(p string) (*regexp.Regexp, bool) {
	if p == "" || strings.HasPrefix(p, "!") || strings.ContainsAny(p, "[]") {
		return nil, false
	}
	anchored := strings.HasPrefix(p, "/") || strings.Contains(strings.TrimSuffix(p, "/"), "/")
	p = strings.TrimPrefix(p, "/")
	folder := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return nil, false
	}
	var re strings.Builder
	re.WriteString("^")
	if !anchored {
		re.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		switch {
		case strings.HasPrefix(p[i:], "**/"):
			re.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(p[i:], "**"):
			re.WriteString(".*")
			i++
		case p[i] == '\\' && i+1 < len(p):
			_, size := utf8.DecodeRuneInString(p[i+1:])
			re.WriteString(regexp.QuoteMeta(p[i+1 : i+1+size]))
			i += size
		case p[i] == '*':
			re.WriteString("[^/]*")
		case p[i] == '?':
			re.WriteString("[^/]")
		default:
			_, size := utf8.DecodeRuneInString(p[i:])
			re.WriteString(regexp.QuoteMeta(p[i : i+size]))
			i += size - 1
		}
	}
	switch {
	case folder:
		re.WriteString("/.*")
	case p == "*", strings.HasSuffix(p, "/*"):
		// GitHub's exception: a trailing /* matches only the files
		// directly in its folder. A bare * is every file anyway.
	default:
		re.WriteString("(?:/.*)?")
	}
	re.WriteString("$")
	compiled, err := regexp.Compile(re.String())
	return compiled, err == nil
}

// Owners are who owns path, from the last rule that matches it: none
// for a path no rule matches, or one a rule without owners matches.
func (c Codeowners) Owners(path string) []string {
	for i := len(c) - 1; i >= 0; i-- {
		if c[i].match.MatchString(path) {
			return c[i].owners
		}
	}
	return nil
}

// Owns reports whether owner owns path. Owners are matched without
// regard to case, and a handle may be given without its @.
func (c Codeowners) Owns(owner, path string) bool {
	owner = NormalOwner(owner)
	for _, o := range c.Owners(path) {
		if strings.EqualFold(o, owner) {
			return true
		}
	}
	return false
}

// NormalOwner is a code owner as CODEOWNERS writes it: a user or team
// handle with its @, or an email.
func NormalOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" || strings.Contains(owner, "@") {
		return owner
	}
	return "@" + owner
}
