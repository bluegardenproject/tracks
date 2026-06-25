package usage

import "strings"

// Pricing is per the claude-api reference, cached 2026-06-04. These
// drift; this table is the single place to update them.
//
// Cache multipliers apply to a model's input price:
//   - read:           ×0.1
//   - write (5m TTL):  ×1.25
//   - write (1h TTL):  ×2.0
const (
	cacheReadMult    = 0.1
	cacheWrite5mMult = 1.25
	cacheWrite1hMult = 2.0
)

// price is USD per million tokens.
type price struct{ in, out float64 }

// priceTable maps a model-name substring → pricing. Matched by
// substring (not exact id) so date-suffixed or context-variant ids
// (e.g. "claude-haiku-4-5-20251001") still resolve. Order matters
// only in that the first hit wins — the families don't overlap.
var priceTable = []struct {
	match string
	price price
}{
	{"opus", price{5, 25}},    // Opus 4.8 / 4.7 / 4.6 / 4.5
	{"sonnet", price{3, 15}},  // Sonnet 4.6
	{"haiku", price{1, 5}},    // Haiku 4.5
	{"fable", price{10, 50}},  // Fable 5
	{"mythos", price{10, 50}}, // Mythos 5 (same as Fable 5)
}

// priceFor returns the input/output per-MTok price for a model. An
// unknown model returns zero pricing — its tokens are still counted,
// but contribute nothing to cost rather than guessing a rate.
func priceFor(model string) (in, out float64) {
	m := strings.ToLower(model)
	for _, e := range priceTable {
		if strings.Contains(m, e.match) {
			return e.price.in, e.price.out
		}
	}
	return 0, 0
}
