package track

// Candor is how bluntly a review is written, from radical candor to
// honest but gently framed, as in v1.
const (
	MinCandor     = 1
	MaxCandor     = 10
	DefaultCandor = 3
)

// candorLabels are v1's, word for word.
var candorLabels = [MaxCandor + 1]string{
	1:  "radical candor — lead with the problem, no cushioning",
	2:  "radical candor — blunt, with minimal framing",
	3:  "direct — plain statements, no hedging",
	4:  "direct — states the problem, adds the why",
	5:  "measured — neutral and even-handed",
	6:  "measured — findings posed as shared problems",
	7:  "diplomatic — leads with what works, findings as suggestions",
	8:  "diplomatic — soft framing, problems posed as questions",
	9:  "gently framed — heavily cushioned, nothing stated flatly",
	10: "gently framed — maximally kind wording, still nothing omitted",
}

// CandorLevel is level, or DefaultCandor when it's out of range.
func CandorLevel(level int) int {
	if level < MinCandor || level > MaxCandor {
		return DefaultCandor
	}
	return level
}

// CandorLabel describes level; out of range, DefaultCandor's.
func CandorLabel(level int) string { return candorLabels[CandorLevel(level)] }
