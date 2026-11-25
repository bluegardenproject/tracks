package workspace

import (
	"fmt"
	"regexp"
	"strings"
)

// Review is what a review track checks out: the ref fetched from origin
// and the name it's shown by.
type Review struct{ Ref, Label string }

// prNumber finds the number in a GitHub pull request URL, with or
// without a scheme, www. prefix, or trailing /files, #anchor or query.
var prNumber = regexp.MustCompile(`^(?:https?://)?(?:www\.)?github\.com/[^/\s]+/[^/\s]+/pull/(\d+)(?:[/?#]\S*)?$`)

// ParseReview reads what the user wants reviewed, as v1 does: a pull
// request URL becomes its pull/<n>/head ref, which lives on the base
// repo's origin even for forks; anything else is a branch on origin.
func ParseReview(ref string) (Review, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Review{}, fmt.Errorf("nothing to review: give a pull request URL or a branch")
	}
	if m := prNumber.FindStringSubmatch(ref); m != nil {
		return Review{Ref: "pull/" + m[1] + "/head", Label: "pr/" + m[1]}, nil
	}
	if strings.Contains(ref, "://") || strings.Contains(ref, "github.com") {
		return Review{}, fmt.Errorf("%q is neither a GitHub pull request URL nor a branch", ref)
	}
	if !validBranchRef(ref) {
		return Review{}, fmt.Errorf("%q is not a valid branch name", ref)
	}
	return Review{Ref: ref, Label: ref}, nil
}

// validBranchRef enforces git-check-ref-format rules and rejects option
// prefixes ('-'), force-refspec prefixes ('+'), and refspec separators (':')
// so a review ref cannot inject git fetch flags or alter refspec handling.
func validBranchRef(ref string) bool {
	if ref == "" || ref == "@" || strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, "+") ||
		strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") ||
		strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") ||
		strings.Contains(ref, "//") || strings.Contains(ref, "@{") {
		return false
	}
	for _, r := range ref {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune(":~^?*\\[", r) {
			return false
		}
	}
	for _, seg := range strings.Split(ref, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".lock") {
			return false
		}
	}
	return true
}
