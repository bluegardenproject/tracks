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

	u, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

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
	u, err := Parse(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if !u.IsZero() {
		t.Errorf("missing file should yield zero usage, got %+v", u)
	}
}

func TestPriceFor(t *testing.T) {
	cases := []struct {
		model   string
		in, out float64
	}{
		{"claude-opus-4-8", 5, 25},
		{"claude-opus-4-6", 5, 25},
		{"claude-sonnet-4-6", 3, 15},
		{"claude-haiku-4-5-20251001", 1, 5}, // date-suffixed id still matches
		{"claude-fable-5", 10, 50},
		{"some-unknown-model", 0, 0}, // unknown → zero, don't guess
	}
	for _, c := range cases {
		in, out := priceFor(c.model)
		if in != c.in || out != c.out {
			t.Errorf("priceFor(%q) = %v/%v, want %v/%v", c.model, in, out, c.in, c.out)
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
