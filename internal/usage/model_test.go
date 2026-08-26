package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShortModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5":             "opus-5",
		"claude-sonnet-5":           "sonnet-5",
		"claude-haiku-4-5-20251001": "haiku-4-5",
		"claude-opus-4-8":           "opus-4-8",
		"claude-fable-5":            "fable-5",
		"  claude-opus-5  ":         "opus-5",
		"":                          "",
		"something-unrecognised":    "something-unrecognised",
		"claude-future-9-20991231":  "future-9",
		// Gateway-hosted ids shorten to the same thing as the direct id,
		// so a Bedrock or Vertex user's MODEL column is just as legible.
		"anthropic.claude-opus-4-20250514-v1:0": "opus-4",
		"claude-opus-4@20250514":                "opus-4",
	}
	for in, want := range cases {
		if got := ShortModel(in); got != want {
			t.Errorf("ShortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// line builds one assistant transcript line.
func line(ts, model string, sidechain bool) string {
	side := "false"
	if sidechain {
		side = "true"
	}
	return `{"type":"assistant","isSidechain":` + side + `,"timestamp":"` + ts +
		`","requestId":"` + ts + `","message":{"model":"` + model +
		`","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
}

func writeTranscript(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	var body string
	for _, l := range lines {
		body += l
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseReportsTheLatestModel(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-17T10:00:00.000Z", "claude-sonnet-5", false),
		line("2026-08-17T10:05:00.000Z", "claude-opus-5", false),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the most recent turn's model", tot.Model)
	}
}

// A Task subagent shares the session id and lands in the same
// transcript. Its model must not be mistaken for the session's.
func TestParseIgnoresSidechainModels(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-17T10:00:00.000Z", "claude-opus-5", false),
		line("2026-08-17T10:06:00.000Z", "claude-haiku-4-5", true),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the main-chain model despite a later subagent turn", tot.Model)
	}
	// The subagent's tokens still count toward the bill.
	if tot.Usage.InputTokens != 20 {
		t.Errorf("InputTokens = %d, want both turns counted (20)", tot.Usage.InputTokens)
	}
}

// Locate can return several transcripts, and glob order is not
// chronological — the newest turn across all of them wins.
func TestParseFilesPicksTheNewestModelAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "a.jsonl")
	newer := filepath.Join(dir, "b.jsonl")
	if err := os.WriteFile(newer, []byte(line("2026-08-17T12:00:00.000Z", "claude-opus-5", false)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(older, []byte(line("2026-08-17T09:00:00.000Z", "claude-sonnet-5", false)), 0o644); err != nil {
		t.Fatal(err)
	}

	// Deliberately pass the newer file first, so "last one scanned wins"
	// would give the wrong answer.
	tot, err := ParseFiles([]string{newer, older})
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the chronologically newest turn's model", tot.Model)
	}
}

func TestParseNoAssistantTurnsYieldsNoModel(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		`{"type":"user","timestamp":"2026-08-17T10:00:00.000Z","message":{"role":"user"}}`+"\n")

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "" {
		t.Errorf("Model = %q, want empty before the first assistant turn", tot.Model)
	}
}

// Claude Code writes "<synthetic>" as the model on an interrupt or an
// API error. Those lines are main-chain and timestamped, so without a
// guard one of them becomes the track's reported model.
func TestParseIgnoresSyntheticModels(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-17T10:00:00.000Z", "claude-opus-5", false),
		line("2026-08-17T10:07:00.000Z", "<synthetic>", false),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the last real model rather than a placeholder", tot.Model)
	}
}

// isRealModel rejects placeholders by shape rather than allow-listing a
// "claude-" prefix, so ids from Bedrock and Vertex — which don't carry
// that prefix in the usual position — are still reported. Without this
// the column would read "no model" forever on a gateway setup.
func TestParseReportsGatewayModelIDs(t *testing.T) {
	for _, id := range []string{
		"anthropic.claude-opus-4-20250514-v1:0",
		"claude-opus-4@20250514",
	} {
		path := writeTranscript(t, "s.jsonl", line("2026-08-17T10:00:00.000Z", id, false))
		tot, err := Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		if tot.Model != id {
			t.Errorf("Model = %q, want %q reported", tot.Model, id)
		}
	}
}

func TestParseReportsSubagentModelSeparately(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-18T10:00:00.000Z", "claude-opus-5", false),
		line("2026-08-18T10:05:00.000Z", "claude-haiku-4-5", true),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the main-chain model", tot.Model)
	}
	if tot.SubagentModel != "claude-haiku-4-5" {
		t.Errorf("SubagentModel = %q, want the sidechain model", tot.SubagentModel)
	}
}

// Last one wins on both chains independently.
func TestParseSubagentModelIsTheMostRecent(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-18T10:00:00.000Z", "claude-sonnet-5", false),
		line("2026-08-18T10:01:00.000Z", "claude-haiku-4-5", true),
		line("2026-08-18T10:02:00.000Z", "claude-opus-4-8", true),
		line("2026-08-18T10:03:00.000Z", "claude-opus-5", false),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want the last main-chain turn", tot.Model)
	}
	if tot.SubagentModel != "claude-opus-4-8" {
		t.Errorf("SubagentModel = %q, want the last sidechain turn", tot.SubagentModel)
	}
}

// A later sub-agent turn must not advance the main model, and vice
// versa — the two chains are tracked independently.
func TestParseChainsDoNotInterfere(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-18T10:00:00.000Z", "claude-opus-5", false),
		line("2026-08-18T11:00:00.000Z", "claude-haiku-4-5", true),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Model != "claude-opus-5" {
		t.Errorf("Model = %q — a later sub-agent turn advanced the main model", tot.Model)
	}
}

func TestParseNoSubagentLeavesItEmpty(t *testing.T) {
	path := writeTranscript(t, "s.jsonl", line("2026-08-18T10:00:00.000Z", "claude-opus-5", false))

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.SubagentModel != "" {
		t.Errorf("SubagentModel = %q, want empty when no sub-agent ran", tot.SubagentModel)
	}
}

// Placeholders are rejected on the sidechain too.
func TestParseIgnoresSyntheticSubagentModels(t *testing.T) {
	path := writeTranscript(t, "s.jsonl",
		line("2026-08-18T10:00:00.000Z", "claude-opus-5", false),
		line("2026-08-18T10:01:00.000Z", "claude-haiku-4-5", true),
		line("2026-08-18T10:02:00.000Z", "<synthetic>", true),
	)

	tot, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if tot.SubagentModel != "claude-haiku-4-5" {
		t.Errorf("SubagentModel = %q, want the last real sub-agent model", tot.SubagentModel)
	}
}
