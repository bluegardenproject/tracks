package usage

import "strings"

// Pricing is per platform.claude.com/docs/en/about-claude/pricing,
// last checked whole on 2026-08-26. The Opus 5.5 row was added and
// verified against that page on 2026-09-22; nothing else was re-read,
// so the older date is what the rest of the table is worth. These
// drift; this table is the single place to update them.
//
// Known gap from the 2026-09-22 visit: Fable 5.1 and Mythos 5.1 read
// cache at ×0.025, and both fall to the ×0.1 default here, overstating
// the cache-read half of a Fable bill 4×. Their per-MTok prices are
// right. Two rows fix it — see cacheRead below.
//
// Cache multipliers apply to a model's input price:
//   - read:           ×0.1, unless the row says otherwise
//   - write (5m TTL):  ×1.25
//   - write (1h TTL):  ×2.0
const (
	cacheReadMult    = 0.1
	cacheWrite5mMult = 1.25
	cacheWrite1hMult = 2.0
)

// price is USD per million tokens. cacheRead overrides cacheReadMult
// for a model whose cache reads are not the usual tenth of its input
// price — Opus 5.5 reads at a twentieth. Zero means the default, so a
// genuinely free cache read is not expressible; no model has one.
type price struct{ in, out, cacheRead float64 }

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
	// These two duplicate their family fallbacks below. Each covers its
	// family's 5 and 5.1 alike, which are priced the same per MTok
	// (their cache reads are not — see the known gap above); they are kept
	// rather than left to the fallback for the same reason sonnet-5 and
	// sonnet-4-6 are both listed: when a differently-priced version
	// ships, the fallback moves to the new price and this row is what
	// keeps the old one correct.
	{"fable-5", price{in: 10, out: 50}},
	{"mythos-5", price{in: 10, out: 50}},

	{"opus-5-5", price{in: 4, out: 20, cacheRead: 0.05}},
	{"opus-5", price{in: 5, out: 25}},
	{"opus-4-8", price{in: 5, out: 25}},
	{"opus-4-7", price{in: 5, out: 25}},
	{"opus-4-6", price{in: 5, out: 25}},
	{"opus-4-5", price{in: 5, out: 25}},

	// Sonnet 5 undercuts Sonnet 4.6 — in this family the newer model is
	// the cheaper one, so "newer costs more" is the wrong intuition.
	// ($2/$10 started as introductory pricing; the increase to $3/$15
	// scheduled for 2026-09-01 was cancelled and it is now standard.)
	{"sonnet-5", price{in: 2, out: 10}},
	{"sonnet-4-6", price{in: 3, out: 15}},
	{"sonnet-4-5", price{in: 3, out: 15}},

	{"haiku-4-5", price{in: 1, out: 5}},

	// Family fallbacks, for a model released after this table was
	// written. Each carries its family's current price on the
	// assumption that a new release is priced like the model it
	// succeeds rather than like a superseded one — a rough number
	// beats a confidently wrong $0.00.
	//
	// Sonnet and Opus are where that assumption is shaky, because two
	// price points coexist in each family: an unlisted "sonnet-4-7"
	// would take Sonnet 5's $2/$10 while actually costing Sonnet 4.6's
	// $3/$15. That is the same coexisting-lines trap this table was
	// rewritten to fix, pointing the other way. It only bites a model
	// that doesn't exist yet, and naming it here is the fix.
	//
	// Opus reaches this row only from "opus-6" up: an unlisted 5.x point
	// release matches the "opus-5" row above and takes Opus 5's $5/$25,
	// not Opus 5.5's $4/$20. The fallback also keeps the default ×0.1
	// cache read rather than inheriting Opus 5.5's ×0.05 exception — for
	// an unknown model, erring high is the same instinct as refusing to
	// price it at $0.00.
	{"fable", price{in: 10, out: 50}},
	{"mythos", price{in: 10, out: 50}},
	{"opus", price{in: 4, out: 20}},
	{"sonnet", price{in: 2, out: 10}},
	{"haiku", price{in: 1, out: 5}},
}

// lookup returns the pricing row for a model, taking the most specific
// entry that matches it. A model from no known family matches nothing
// and gets the zero row: its tokens are still counted, but contribute
// nothing to cost rather than guessing a rate.
func lookup(model string) (price, bool) {
	m := strings.ToLower(model)
	longest := -1
	var p price
	for _, e := range priceTable {
		if len(e.match) > longest && strings.Contains(m, e.match) {
			longest, p = len(e.match), e.price
		}
	}
	return p, longest >= 0
}

// priceFor returns the input/output per-MTok price for a model. Cost
// is computed from lookup directly, because it needs the cache-read
// multiplier off the same row; this is the table's readable two-number
// view, and what the tests assert against.
func priceFor(model string) (in, out float64) {
	p, ok := lookup(model)
	if !ok {
		return 0, 0
	}
	return p.in, p.out
}

// cacheReadMultiplier returns what a cache read costs as a fraction of
// this row's input price.
func (p price) cacheReadMultiplier() float64 {
	if p.cacheRead > 0 {
		return p.cacheRead
	}
	return cacheReadMult
}
