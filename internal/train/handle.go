package train

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
)

// A train's handle is its prefix and a number of handleDigits digits,
// new each time a train starts: reviewer-527192.
const handleDigits = 6

// MaxPrefix leaves a prefix room for "-" and the number within a
// handle's 32 characters.
const MaxPrefix = 32 - 1 - handleDigits

var prefixPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// reserved are the relay's own handles, which no train takes.
var reserved = []string{"track", "tracks"}

// CheckPrefix says why p can't start trains' handles, nil when it can:
// a lower-case letter, then lower-case letters, digits and hyphens, at
// most MaxPrefix in all.
func CheckPrefix(p string) error {
	if !prefixPattern.MatchString(p) || len(p) > MaxPrefix || strings.HasSuffix(p, "-") {
		return fmt.Errorf("%q isn't a prefix for trains: a lower-case letter, then up to %d lower-case letters, "+
			"digits and hyphens, not ending in one", p, MaxPrefix-1)
	}
	if slices.Contains(reserved, p) {
		return fmt.Errorf("%q is the relay's own", p)
	}
	return nil
}

// NewHandle is a handle for a train with prefix that taken doesn't
// hold: prefix, "-", and handleDigits random digits.
func NewHandle(prefix string, taken func(string) bool) (string, error) {
	if err := CheckPrefix(prefix); err != nil {
		return "", err
	}
	limit := big.NewInt(1_000_000)
	for range 100 {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		h := fmt.Sprintf("%s-%0*d", prefix, handleDigits, n.Int64())
		if !taken(h) {
			return h, nil
		}
	}
	return "", fmt.Errorf("no free handle for %s", prefix)
}

// Resolve is the handle among handles that name means: the one it is,
// or the one whose prefix it is when only one has it, as `reviewer` for
// reviewer-527192. A leading @, as messages show handles, is dropped.
func Resolve(name string, handles []string) (string, error) {
	name = strings.TrimPrefix(name, "@")
	if slices.Contains(handles, name) {
		return name, nil
	}
	var matches []string
	for _, h := range handles {
		if rest, ok := strings.CutPrefix(h, name+"-"); ok && isNumber(rest) {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no train called %q", name)
	}
	return "", fmt.Errorf("%s could be %s: name one", name, strings.Join(matches, " or "))
}

func isNumber(s string) bool {
	if len(s) != handleDigits {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
