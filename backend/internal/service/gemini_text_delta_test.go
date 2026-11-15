package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyComputeGeminiTextDelta is the previous string-concatenating
// implementation, kept as the oracle for the storage-only change.
func legacyComputeGeminiTextDelta(seen, incoming string) (delta, newSeen string) {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return "", seen
	}
	if strings.HasPrefix(incoming, seen) {
		return strings.TrimPrefix(incoming, seen), incoming
	}
	if strings.HasPrefix(seen, incoming) {
		return "", seen
	}
	return incoming, seen + incoming
}

func TestGeminiTextDeltaMatchesLegacy(t *testing.T) {
	sequences := [][]string{
		{"Hel", "Hello", "Hello, wor", "Hello, world"},   // cumulative
		{"Hel", "lo", ", wor", "ld"},                     // incremental
		{"The", " cat", "The", " sat"},                   // delta equal to an emitted prefix
		{"abc", "abc", "ab", "abcd", "x"},                // duplicate, rewind, extend, diverge
		{"", "\u0000", "a\u0000", "ab", "", "abc\u0000"}, // empty and NUL-terminated chunks
		{"{\"a\":", "{\"a\":1", "}", "{\"a\":1}"},        // tool args mixing modes
		{"多字节", "多字节文本", "😀", "多字节文本😀继续"},                // non-ASCII
	}
	for _, seq := range sequences {
		var state geminiTextDelta
		legacySeen := ""
		for i, chunk := range seq {
			wantDelta, newSeen := legacyComputeGeminiTextDelta(legacySeen, chunk)
			legacySeen = newSeen
			require.Equal(t, wantDelta, state.next(chunk), "sequence %q chunk %d", seq, i)
			require.Equal(t, legacySeen, state.seen.String(), "sequence %q chunk %d", seq, i)
		}
		state.reset()
		require.Equal(t, "", state.seen.String())
		require.Equal(t, "fresh", state.next("fresh"))
	}
}

func TestGeminiTextDeltaIncrementalAllocationsStayLinear(t *testing.T) {
	chunk := strings.Repeat("x", 100)
	const chunks = 2000
	allocs := testing.AllocsPerRun(5, func() {
		var state geminiTextDelta
		for i := 0; i < chunks; i++ {
			_ = state.next(chunk)
		}
	})
	// Concatenation allocates once per chunk; amortized growth needs O(log n).
	require.Less(t, allocs, float64(100), "allocs per stream: %v", allocs)
}
