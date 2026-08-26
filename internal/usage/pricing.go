package usage

import "strings"

// Pricing is per platform.claude.com/docs/en/about-claude/pricing,
// checked 2026-08-26. These drift; this table is the single place to
// update them.
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

// priceEntry is one row of priceTable: the substring to look for and
// what a model matching it costs.
type priceEntry struct {
	match string
	price price
}

// priceTable maps a model-name substring → pricing, resolved by the
// LONGEST entry that occurs in the id, so a version-specific row wins
// over the family fallback below it: "claude-sonnet-5" is priced from
// the "sonnet-5" row, not from "sonnet". Order is for reading only.
//
// Matching on a substring rather than the exact id keeps the
// gateway-hosted and date-suffixed spellings working:
// "anthropic.claude-sonnet-5" (Bedrock), "claude-haiku-4-5@20251001"
// (Vertex), "claude-haiku-4-5-20251001".
//
// Retired models (Opus 4 and 4.1, Sonnet 4, Haiku 3.5) are left out on
// purpose. Claude Code will not start a session on one, and their
// "opus-4" / "sonnet-4" shaped keys would shadow every future point
// release in those families — "claude-opus-4-9" would silently pick up
// retired Opus 4's $15/$75. Legacy version-first ids
// ("claude-3-5-sonnet-…") misprice against the sonnet fallback for the
// same reason, and are out of scope for the same one.
var priceTable = []priceEntry{
	// These two duplicate their family fallbacks below, because each
	// family currently has exactly one version. They are kept rather
	// than left to the fallback for the same reason sonnet-5 and
	// sonnet-4-6 are both listed: when a second version ships, the
	// fallback moves to the new price and this row is what keeps the
	// old one correct.
	{"fable-5", price{10, 50}},
	{"mythos-5", price{10, 50}},

	{"opus-5", price{5, 25}},
	{"opus-4-8", price{5, 25}},
	{"opus-4-7", price{5, 25}},
	{"opus-4-6", price{5, 25}},
	{"opus-4-5", price{5, 25}},

	// Sonnet 5 undercuts Sonnet 4.6 — in this family the newer model is
	// the cheaper one, so "newer costs more" is the wrong intuition.
	// ($2/$10 started as introductory pricing; the increase to $3/$15
	// scheduled for 2026-09-01 was cancelled and it is now standard.)
	{"sonnet-5", price{2, 10}},
	{"sonnet-4-6", price{3, 15}},
	{"sonnet-4-5", price{3, 15}},

	{"haiku-4-5", price{1, 5}},

	// Family fallbacks, for a model released after this table was
	// written. Each carries its family's current price on the
	// assumption that a new release is priced like the model it
	// succeeds rather than like a superseded one — a rough number
	// beats a confidently wrong $0.00.
	//
	// Sonnet is the one place that assumption is shaky, because two
	// price points coexist in the family: an unlisted "sonnet-4-7"
	// would take Sonnet 5's $2/$10 while actually costing Sonnet 4.6's
	// $3/$15. That is the same coexisting-lines trap this table was
	// rewritten to fix, pointing the other way. It only bites a model
	// that doesn't exist yet, and naming it here is the fix.
	{"fable", price{10, 50}},
	{"mythos", price{10, 50}},
	{"opus", price{5, 25}},
	{"sonnet", price{2, 10}},
	{"haiku", price{1, 5}},
}

// priceFor returns the input/output per-MTok price for a model, taking
// the most specific entry that matches it. A model from no known
// family returns zero pricing — its tokens are still counted, but
// contribute nothing to cost rather than guessing a rate.
func priceFor(model string) (in, out float64) {
	m := strings.ToLower(model)
	longest := -1
	var p price
	for _, e := range priceTable {
		if len(e.match) > longest && strings.Contains(m, e.match) {
			longest, p = len(e.match), e.price
		}
	}
	if longest < 0 {
		return 0, 0
	}
	return p.in, p.out
}
