// Package ports assigns each track a private, reproducible block of TCP
// ports for its dev servers.
//
// The block lives high in the ephemeral range so it never collides with
// the default ports manual dev servers use (Metro 8081, webpack 3000,
// Vite 5173, …). Allocation is deterministic in the track ID — the same
// ID always prefers the same block — and skips ports already handed out
// to other live tracks, so two concurrent tracks running the same
// services don't fight over a port.
package ports

import (
	"errors"
	"fmt"
	"hash/fnv"
)

const (
	// RangeStart and RangeEnd bound the allocatable space. Starts high so
	// manual default-port servers are never touched.
	RangeStart = 20000
	RangeEnd   = 60000

	// BlockSize is how many contiguous ports each track reserves. One per
	// service, with headroom; also caps services per track.
	BlockSize = 50
)

// Allocate assigns a port to each name, returning a map keyed by name.
// Names get contiguous ports within a single block. The block is chosen
// from trackID — the same id always *prefers* the same block — but when
// that block's ports appear in taken (the flattened Ports of all live
// tracks) allocation walks forward to the next free block, so the result
// is reproducible given the same taken set but not invariant to it. The
// returned map is empty (nil-safe to range) when names is empty.
func Allocate(trackID string, names []string, taken map[int]bool) (map[string]int, error) {
	if len(names) == 0 {
		return map[string]int{}, nil
	}
	if len(names) > BlockSize {
		return nil, fmt.Errorf("too many services (%d); max %d per track", len(names), BlockSize)
	}
	nBlocks := (RangeEnd - RangeStart) / BlockSize
	start := int(hashID(trackID) % uint64(nBlocks))
	for i := 0; i < nBlocks; i++ {
		base := RangeStart + ((start+i)%nBlocks)*BlockSize
		if !blockFree(base, len(names), taken) {
			continue
		}
		out := make(map[string]int, len(names))
		for j, name := range names {
			out[name] = base + j
		}
		return out, nil
	}
	return nil, errors.New("no free port block available")
}

// blockFree reports whether the first n ports from base are all unused.
func blockFree(base, n int, taken map[int]bool) bool {
	for k := 0; k < n; k++ {
		if taken[base+k] {
			return false
		}
	}
	return true
}

// hashID is a stable 64-bit hash of the track ID, so allocation is
// reproducible across daemon restarts.
func hashID(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
