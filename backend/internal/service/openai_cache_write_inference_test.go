package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClassifyOpenAICacheWriteField(t *testing.T) {
	t.Parallel()

	require.Equal(t, openAICacheWriteFieldPositive, classifyOpenAICacheWriteField(false, 123))
	require.Equal(t, openAICacheWriteFieldAbsent, classifyOpenAICacheWriteField(false, 0))
	require.Equal(t, openAICacheWriteFieldExplicitZero, classifyOpenAICacheWriteField(true, 0))
	require.Equal(t, openAICacheWriteFieldExplicitZero, classifyOpenAICacheWriteField(true, -1))
	require.Equal(t, openAICacheWriteFieldPositive, classifyOpenAICacheWriteField(true, 7))
}

func TestInferOpenAICacheWriteFromNextHit_SubscriptionExample(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	previous := openAICacheWriteObservation{
		AccountID:       11,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-a",
		Sequence:        1,
		InputTokens:     10818,
		CacheReadTokens: 0,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}
	next := openAICacheWriteObservation{
		AccountID:       11,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-a",
		Sequence:        2,
		InputTokens:     10818,
		CacheReadTokens: 9984,
		ObservedAt:      start.Add(time.Minute),
	}

	got, ok := inferOpenAICacheWriteFromNextHit(previous, next, 0)
	require.True(t, ok)
	require.Equal(t, 9984, got.Tokens)
	require.Equal(t, 834, got.ResidualInputTokens)
	require.Equal(t, 0, got.PreviousCacheRead)
	require.Equal(t, 9984, got.NextCacheRead)
	require.Equal(t, "inferred_next_hit", got.Source)
}

func TestInferOpenAICacheWriteFromNextHit_ExplainsFixed68Residual(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	previous := openAICacheWriteObservation{
		AccountID:       12,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-b",
		Sequence:        41,
		InputTokens:     154668,
		CacheReadTokens: 152400,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}
	next := openAICacheWriteObservation{
		AccountID:       12,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-b",
		Sequence:        42,
		InputTokens:     156900,
		CacheReadTokens: 154600,
		ObservedAt:      start.Add(20 * time.Second),
	}

	got, ok := inferOpenAICacheWriteFromNextHit(previous, next, 0)
	require.True(t, ok)
	require.Equal(t, 2200, got.Tokens)
	require.Equal(t, 68, got.ResidualInputTokens)
}

func TestInferOpenAICacheWriteFromNextHit_DoesNotOverrideAuthoritativeField(t *testing.T) {
	t.Parallel()

	base := openAICacheWriteObservation{
		AccountID:       13,
		Model:           "gpt-6-sol",
		CacheIdentity:   "session-c",
		Sequence:        7,
		InputTokens:     12000,
		CacheReadTokens: 9000,
		ObservedAt:      time.Now(),
	}
	next := base
	next.Sequence = 8
	next.CacheReadTokens = 11000
	next.ObservedAt = base.ObservedAt.Add(time.Second)

	for _, state := range []openAICacheWriteFieldState{
		openAICacheWriteFieldExplicitZero,
		openAICacheWriteFieldPositive,
	} {
		previous := base
		previous.CacheWriteState = state
		_, ok := inferOpenAICacheWriteFromNextHit(previous, next, 0)
		require.False(t, ok)
	}
}

func TestInferOpenAICacheWriteFromNextHit_RejectsUnsafeLineage(t *testing.T) {
	t.Parallel()

	now := time.Now()
	base := openAICacheWriteObservation{
		AccountID:       20,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-x",
		Sequence:        10,
		InputTokens:     20000,
		CacheReadTokens: 15000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      now,
	}
	validNext := openAICacheWriteObservation{
		AccountID:       20,
		Model:           "gpt-6-astra",
		CacheIdentity:   "session-x",
		Sequence:        11,
		InputTokens:     22000,
		CacheReadTokens: 18000,
		ObservedAt:      now.Add(time.Minute),
	}

	tests := []struct {
		name   string
		mutate func(*openAICacheWriteObservation, *openAICacheWriteObservation)
	}{
		{"account mismatch", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.AccountID++ }},
		{"model mismatch", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.Model = "gpt-6-sol" }},
		{"identity mismatch", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.CacheIdentity = "other" }},
		{"missing identity", func(p *openAICacheWriteObservation, _ *openAICacheWriteObservation) { p.CacheIdentity = "" }},
		{"non-adjacent sequence", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.Sequence = 12 }},
		{"cache read decreased", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.CacheReadTokens = 14000 }},
		{"cache read unchanged", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.CacheReadTokens = 15000 }},
		{"write exceeds previous uncached input", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.CacheReadTokens = 21000 }},
		{"invalid previous total", func(p *openAICacheWriteObservation, _ *openAICacheWriteObservation) { p.InputTokens = 14000 }},
		{"invalid next total", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.InputTokens = 17000 }},
		{"shrinking total input", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.InputTokens = 19000 }},
		{"stale observation", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.ObservedAt = now.Add(31 * time.Minute) }},
		{"time reversed", func(_ *openAICacheWriteObservation, n *openAICacheWriteObservation) { n.ObservedAt = now.Add(-time.Second) }},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			previous := base
			next := validNext
			tc.mutate(&previous, &next)
			_, ok := inferOpenAICacheWriteFromNextHit(previous, next, 0)
			require.False(t, ok)
		})
	}
}
