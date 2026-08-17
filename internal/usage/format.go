package usage

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FormatCost renders a USD cost compactly: "$3.45", "$12.30". A
// nonzero cost below a cent shows "<$0.01" rather than rounding to
// "$0.00"; an exactly-zero cost shows "$0.00".
func FormatCost(c float64) string {
	switch {
	case c <= 0:
		return "$0.00"
	case c < 0.01:
		return "<$0.01"
	default:
		return fmt.Sprintf("$%.2f", c)
	}
}

// FormatTokens renders a token count compactly: 517 → "517",
// 42_000 → "42.0K", 1_180_000 → "1.18M".
func FormatTokens(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
}

// FormatDuration renders a wall-clock span compactly: "45s", "12m",
// "1h3m". Seconds are dropped once it's a minute or more.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// modelDateSuffix matches the release-date tail some model ids carry,
// in either separator style and with Bedrock's trailing version:
// claude-haiku-4-5-20251001, claude-opus-4@20250514 (Vertex),
// anthropic.claude-opus-4-20250514-v1:0 (Bedrock).
var modelDateSuffix = regexp.MustCompile(`[-@]\d{8}(-v\d+:\d+)?$`)

// ShortModel renders a model id for a narrow column: the vendor prefix
// and any release-date suffix come off, so `claude-haiku-4-5-20251001`
// reads as `haiku-4-5` and `claude-opus-5` as `opus-5`. Gateway-hosted
// ids shorten to the same thing as their direct equivalents —
// `anthropic.claude-opus-4-20250514-v1:0` and `claude-opus-4@20250514`
// both give `opus-4` — so a Bedrock user's column stays as legible as
// anyone else's instead of every row reading `anthropic.…`.
//
// An unrecognised id is returned trimmed but otherwise untouched:
// better to show something odd than to show nothing.
func ShortModel(id string) string {
	s := strings.TrimSpace(id)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "anthropic.") // Bedrock
	s = strings.TrimPrefix(s, "claude-")
	return modelDateSuffix.ReplaceAllString(s, "")
}
