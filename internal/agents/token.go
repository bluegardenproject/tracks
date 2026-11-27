package agents

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// TokenEnv is the variable an agent's pane holds its token in, beside
// TRACKS_ID: tracks mcp shows it to the daemon, which knows the agent
// by it.
const TokenEnv = "TRACKS_AGENT_TOKEN"

// NewToken is a fresh agent token: 32 random bytes, in hex.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken is what Tracks keeps of token: its SHA-256, in hex. A token
// is random enough that a plain hash can't be reversed.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
