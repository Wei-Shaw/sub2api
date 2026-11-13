package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

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
	// AllocsPerRun reads process-wide counters. Background goroutines left by
	// other service tests can allocate even when it sets GOMAXPROCS to one.
	const childEnv = "SUB2API_GEMINI_ALLOC_TEST_CHILD"
	if os.Getenv(childEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		executable, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.CommandContext(ctx, executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "isolated allocation test failed:\n%s", output)
		return
	}

	const chunks = 2000
	inputs := make([]string, chunks)
	for i := range inputs {
		// Repeating the same chunk exercises duplicate suppression instead of
		// growing the buffer. A unique prefix makes every chunk incremental.
		inputs[i] = fmt.Sprintf("%04d:%s", i, strings.Repeat("x", 95))
	}
	want := strings.Join(inputs, "")
	var got string
	allocs := testing.AllocsPerRun(5, func() {
		var state geminiTextDelta
		for _, chunk := range inputs {
			_ = state.next(chunk)
		}
		got = state.seen.String()
	})
	require.Equal(t, want, got, "all incremental chunks must be retained")
	// Concatenation allocates once per chunk; amortized growth needs O(log n).
	require.Less(t, allocs, float64(100), "allocs per stream: %v", allocs)
}
