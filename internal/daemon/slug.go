package daemon

import (
	"regexp"
	"strings"
)

// ticketRE matches Jira-style ticket references (e.g. LIVE-1234,
// ABC-99). We deliberately require at least two characters in the
// project prefix so common words ("a-3", "i-5") don't false-match.
var ticketRE = regexp.MustCompile(`\b[A-Z][A-Z0-9_]+-\d+\b`)

// stopwordsForSlug are common English filler words we strip when
// building the descriptive portion of an auto-generated branch
// slug. Keeping them produces ugly branches like
// "fix/please-fix-the-bug-on" — strip them and we get
// "fix/bug-fix". Conservative list: only obvious noise.
var stopwordsForSlug = map[string]bool{
	"a": true, "an": true, "the": true,
	"and": true, "or": true, "but": true,
	"to": true, "of": true, "in": true, "on": true, "at": true,
	"for": true, "with": true, "as": true, "by": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"do": true, "does": true, "did": true,
	"please": true, "can": true, "could": true, "should": true, "would": true,
	"this": true, "that": true,
	"i": true, "you": true, "we": true, "they": true, "it": true,
	"my": true, "our": true, "your": true,
}

// maxSlugLength bounds the slug portion (after the optional ticket
// prefix) so branch names stay readable in `git log --oneline`.
const maxSlugLength = 40

// deriveSlugFromTask builds a branch slug from the user's task
// prompt. Returns "" only when the prompt is empty — every other
// input produces something usable, even if it's just the track ID
// suffix passed in as the fallback.
//
// Algorithm:
//
//  1. Pull out the first Jira-style ticket reference (if any).
//  2. Tokenize the prompt, drop stopwords + the ticket itself,
//     keep alphanumeric, lowercase, hyphen-join.
//  3. Trim to maxSlugLength.
//  4. Combine ticket and descriptive part as TICKET-descriptive.
//  5. If both are empty, fall back to the supplied id suffix.
func deriveSlugFromTask(task, fallback string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return fallback
	}

	ticket := ticketRE.FindString(task)

	// Strip the ticket from the source text before tokenizing.
	// Otherwise tokenization splits "LIVE-1234" into "LIVE" + "1234"
	// (hyphens become spaces) and those fragments end up duplicated
	// in the descriptive part.
	taskForWords := task
	if ticket != "" {
		taskForWords = strings.ReplaceAll(taskForWords, ticket, " ")
	}

	// Tokenize. Replace anything that isn't alphanumeric with a
	// space, then split on whitespace. This handles punctuation,
	// quotes, parentheses, etc. without a complex regex.
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return ' '
		}
	}, taskForWords)

	words := strings.Fields(clean)
	descriptiveParts := make([]string, 0, 6)
	for _, w := range words {
		lower := strings.ToLower(w)
		if stopwordsForSlug[lower] {
			continue
		}
		descriptiveParts = append(descriptiveParts, lower)
		if len(descriptiveParts) >= 5 {
			break
		}
	}
	// Join word-by-word, never mid-word. If adding the next word
	// would overflow maxSlugLength, stop early.
	var built []string
	total := 0
	for _, p := range descriptiveParts {
		sep := 0
		if len(built) > 0 {
			sep = 1
		}
		if total+sep+len(p) > maxSlugLength {
			break
		}
		built = append(built, p)
		total += sep + len(p)
	}
	descriptive := strings.Join(built, "-")

	switch {
	case ticket != "" && descriptive != "":
		return ticket + "-" + descriptive
	case ticket != "":
		return ticket
	case descriptive != "":
		return descriptive
	default:
		return fallback
	}
}
