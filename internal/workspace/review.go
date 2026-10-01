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
// without a trailing /files, #anchor or query.
var prNumber = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/(\d+)`)

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
	return Review{Ref: ref, Label: ref}, nil
}
