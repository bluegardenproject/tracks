package usage

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// One assistant turn on Opus (priced 5/25), one on Haiku (1/5), a
// non-assistant line that must be ignored, and a duplicate requestId
// that must be deduped.
const fixture = `{"type":"assistant","requestId":"A","message":{"model":"claude-opus-4-8","usage":{"input_tokens":1000000,"output_tokens":200000,"cache_read_input_tokens":500000,"cache_creation_input_tokens":100000,"cache_creation":{"ephemeral_5m_input_tokens":100000,"ephemeral_1h_input_tokens":0}}}}
{"type":"assistant","requestId":"B","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":2000000,"output_tokens":100000}}}
{"type":"user","message":{"role":"user"}}
{"type":"assistant","requestId":"B","message":{"model":"claude-haiku-4-5","usage":{"input_tokens":2000000,"output_tokens":100000}}}
`

func TestParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	u := tot.Usage

	if u.InputTokens != 3_000_000 {
		t.Errorf("InputTokens = %d, want 3000000", u.InputTokens)
	}
	if u.OutputTokens != 300_000 {
		t.Errorf("OutputTokens = %d, want 300000", u.OutputTokens)
	}
	if u.CacheReadTokens != 500_000 {
		t.Errorf("CacheReadTokens = %d, want 500000", u.CacheReadTokens)
	}
	if u.CacheCreationTokens != 100_000 {
		t.Errorf("CacheCreationTokens = %d, want 100000", u.CacheCreationTokens)
	}
	// Opus: 1e6*5 + 5e5*5*0.1 + 1e5*5*1.25 + 2e5*25 = 10.875
	// Haiku: 2e6*1 + 1e5*5 = 2.5  → total 13.375 (counted once, deduped)
	if math.Abs(u.CostUSD-13.375) > 1e-9 {
		t.Errorf("CostUSD = %v, want 13.375", u.CostUSD)
	}
}

func TestParseMissingFileIsZero(t *testing.T) {
	tot, err := Parse(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	u := tot.Usage
	if !u.IsZero() {
		t.Errorf("missing file should yield zero usage, got %+v", u)
	}
}

// Every currently-available model, by the exact id the transcript
// records. Checked against the pricing docs on 2026-08-26 — when a
// price moves, this table and priceTable move together.
func TestPriceFor(t *testing.T) {
	cases := []struct {
		model   string
		in, out float64
	}{
		{"claude-fable-5", 10, 50},
		{"claude-mythos-5", 10, 50},
		{"claude-opus-5", 5, 25},
		{"claude-opus-4-8", 5, 25},
		{"claude-opus-4-7", 5, 25},
		{"claude-opus-4-6", 5, 25},
		{"claude-opus-4-5", 5, 25},
		{"claude-sonnet-5", 2, 10},
		{"claude-sonnet-4-6", 3, 15},
		{"claude-sonnet-4-5", 3, 15},
		{"claude-haiku-4-5", 1, 5},
		{"claude-haiku-4-5-20251001", 1, 5},  // date-suffixed id still matches
		{"anthropic.claude-sonnet-5", 2, 10}, // Bedrock
		{"claude-haiku-4-5@20251001", 1, 5},  // Vertex
		{"some-unknown-model", 0, 0},         // no known family → zero, don't guess
		// Legacy version-first ids are knowingly mispriced: this one is
		// really $3/$15, but the family fallback is all that matches.
		// Pinned so the documented wrong answer can't drift silently —
		// the model is retired, so the fix is not worth a table entry.
		{"claude-3-5-sonnet-20241022", 2, 10},
	}
	for _, c := range cases {
		in, out := priceFor(c.model)
		if in != c.in || out != c.out {
			t.Errorf("priceFor(%q) = %v/%v, want %v/%v", c.model, in, out, c.in, c.out)
		}
	}
}

// The bug this table shape exists to prevent: a plain substring match
// on "sonnet" priced Sonnet 5 at Sonnet 4.6's rate, overstating every
// Sonnet 5 track's cost by 50%. The two must resolve apart, and the
// newer one must be the cheaper.
func TestSonnetVersionsArePricedApart(t *testing.T) {
	in5, out5 := priceFor("claude-sonnet-5")
	in46, out46 := priceFor("claude-sonnet-4-6")
	if in5 == in46 && out5 == out46 {
		t.Fatalf("sonnet-5 and sonnet-4-6 both priced %v/%v — the version entry is not winning over the family fallback", in5, out5)
	}
	if in5 >= in46 || out5 >= out46 {
		t.Errorf("sonnet-5 (%v/%v) should undercut sonnet-4-6 (%v/%v)", in5, out5, in46, out46)
	}
}

// A model released after this table was written still has to cost
// something — a silent $0.00 reads as "this track was free" rather
// than "tracks doesn't know this model yet".
func TestUnknownVersionFallsBackToItsFamily(t *testing.T) {
	cases := []struct {
		model   string
		in, out float64
	}{
		{"claude-opus-9", 5, 25},
		{"claude-sonnet-9", 2, 10},
		{"claude-haiku-9", 1, 5},
		{"claude-fable-9", 10, 50},
	}
	for _, c := range cases {
		in, out := priceFor(c.model)
		if in != c.in || out != c.out {
			t.Errorf("priceFor(%q) = %v/%v, want the family rate %v/%v", c.model, in, out, c.in, c.out)
		}
	}
}

// Resolution must depend on how specific an entry is, not on where it
// sits in the table — every family key is a substring of the version
// keys beneath it, so a first-match-wins lookup would be correct only
// as long as nobody reorders the rows. Reversing the table (families
// first) has to change nothing.
func TestSpecificityBeatsTableOrder(t *testing.T) {
	ids := []string{
		"claude-sonnet-5", "claude-sonnet-4-6", "claude-sonnet-4-5",
		"claude-opus-5", "claude-opus-4-8", "claude-haiku-4-5",
		"claude-fable-5", "claude-mythos-5",
	}
	want := map[string][2]float64{}
	for _, id := range ids {
		in, out := priceFor(id)
		want[id] = [2]float64{in, out}
	}

	// Swapping a package-level var is safe only while no test in this
	// package runs in parallel. Nothing here calls t.Parallel(); if that
	// changes, pass the table into a lookup helper instead.
	orig := priceTable
	defer func() { priceTable = orig }()
	reversed := make([]priceEntry, 0, len(orig))
	for i := len(orig) - 1; i >= 0; i-- {
		reversed = append(reversed, orig[i])
	}
	priceTable = reversed

	for _, id := range ids {
		in, out := priceFor(id)
		if [2]float64{in, out} != want[id] {
			t.Errorf("priceFor(%q) = %v/%v with the table reversed, was %v/%v — lookup depends on row order",
				id, in, out, want[id][0], want[id][1])
		}
	}
}

func TestSanitizeCWD(t *testing.T) {
	got := sanitizeCWD("/Users/x/.local/state/tracks/worktrees/abc/repo")
	want := "-Users-x--local-state-tracks-worktrees-abc-repo"
	if got != want {
		t.Errorf("sanitizeCWD = %q, want %q", got, want)
	}
}

func TestFormatters(t *testing.T) {
	if got := FormatCost(0); got != "$0.00" {
		t.Errorf("FormatCost(0) = %q", got)
	}
	if got := FormatCost(0.004); got != "<$0.01" {
		t.Errorf("FormatCost(0.004) = %q", got)
	}
	if got := FormatCost(3.456); got != "$3.46" {
		t.Errorf("FormatCost(3.456) = %q", got)
	}
	if got := FormatTokens(517); got != "517" {
		t.Errorf("FormatTokens(517) = %q", got)
	}
	if got := FormatTokens(42_000); got != "42.0K" {
		t.Errorf("FormatTokens(42000) = %q", got)
	}
	if got := FormatTokens(1_180_000); got != "1.18M" {
		t.Errorf("FormatTokens(1180000) = %q", got)
	}
	if got := FormatDuration(45 * time.Second); got != "45s" {
		t.Errorf("FormatDuration(45s) = %q", got)
	}
	if got := FormatDuration(12 * time.Minute); got != "12m" {
		t.Errorf("FormatDuration(12m) = %q", got)
	}
	if got := FormatDuration(63 * time.Minute); got != "1h3m" {
		t.Errorf("FormatDuration(63m) = %q", got)
	}
}
