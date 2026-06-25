package usage

import (
	"fmt"
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
