package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAICacheWriteInferenceTracker_InfersAdjacentTurn(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}

	first := openAICacheWriteObservation{
		AccountID:       7,
		Model:           "gpt-6-astra",
		CacheIdentity:   "cache-a",
		InputTokens:     154668,
		CacheReadTokens: 152400,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}
	_, _, ok := tracker.observe(first, "req-1")
	require.False(t, ok)

	second := openAICacheWriteObservation{
		AccountID:       7,
		Model:           "gpt-6-astra",
		CacheIdentity:   "cache-a",
		InputTokens:     156900,
		CacheReadTokens: 154600,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start.Add(time.Second),
	}
	got, previousRequestID, ok := tracker.observe(second, "req-2")
	require.True(t, ok)
	require.Equal(t, "req-1", previousRequestID)
	require.Equal(t, 2200, got.Tokens)
	require.Equal(t, 68, got.ResidualInputTokens)
}

func TestOpenAICacheWriteInferenceTracker_DuplicateRequestDoesNotAdvance(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}
	first := openAICacheWriteObservation{
		AccountID:       8,
		Model:           "gpt-6-astra",
		CacheIdentity:   "cache-b",
		InputTokens:     10000,
		CacheReadTokens: 8000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}
	tracker.observe(first, "same-request")

	duplicate := first
	duplicate.CacheReadTokens = 9000
	duplicate.ObservedAt = start.Add(time.Second)
	_, _, ok := tracker.observe(duplicate, "same-request")
	require.False(t, ok)

	next := openAICacheWriteObservation{
		AccountID:       8,
		Model:           "gpt-6-astra",
		CacheIdentity:   "cache-b",
		InputTokens:     11000,
		CacheReadTokens: 9000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start.Add(2 * time.Second),
	}
	got, _, ok := tracker.observe(next, "req-next")
	require.True(t, ok)
	require.Equal(t, 1000, got.Tokens)
	require.Equal(t, 1000, got.ResidualInputTokens)
}

func TestOpenAICacheWriteInferenceTracker_StaleTurnResetsLineage(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}
	tracker.observe(openAICacheWriteObservation{
		AccountID:       9,
		Model:           "gpt-6-sol",
		CacheIdentity:   "cache-c",
		InputTokens:     10000,
		CacheReadTokens: 5000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}, "req-old")

	_, _, ok := tracker.observe(openAICacheWriteObservation{
		AccountID:       9,
		Model:           "gpt-6-sol",
		CacheIdentity:   "cache-c",
		InputTokens:     12000,
		CacheReadTokens: 7000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start.Add(defaultOpenAICacheWriteInferenceWindow + time.Second),
	}, "req-new")
	require.False(t, ok)
}


func TestOpenAICacheWriteInferenceTracker_RebaselineAfterMiss(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}

	turns := []openAICacheWriteObservation{
		{
			AccountID: 10, Model: "gpt-6-astra", CacheIdentity: "cache-d",
			InputTokens: 10000, CacheReadTokens: 0,
			CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start,
		},
		{
			AccountID: 10, Model: "gpt-6-astra", CacheIdentity: "cache-d",
			InputTokens: 11000, CacheReadTokens: 0,
			CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(time.Second),
		},
		{
			AccountID: 10, Model: "gpt-6-astra", CacheIdentity: "cache-d",
			InputTokens: 12000, CacheReadTokens: 1000,
			CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(2 * time.Second),
		},
		{
			AccountID: 10, Model: "gpt-6-astra", CacheIdentity: "cache-d",
			InputTokens: 13000, CacheReadTokens: 2000,
			CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(3 * time.Second),
		},
	}

	_, _, ok := tracker.observe(turns[0], "req-1")
	require.False(t, ok)

	_, _, ok = tracker.observe(turns[1], "req-2")
	require.False(t, ok, "a miss cannot prove a zero write")

	_, _, ok = tracker.observe(turns[2], "req-3")
	require.False(t, ok, "first hit after a miss is ambiguous and only rebaselines")

	got, previousRequestID, ok := tracker.observe(turns[3], "req-4")
	require.True(t, ok)
	require.Equal(t, "req-3", previousRequestID)
	require.Equal(t, 1000, got.Tokens)
	require.Equal(t, 10000, got.ResidualInputTokens)
}


func TestOpenAICacheWriteInferenceTracker_RebaselineAfterOutOfOrderShrink(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}

	turn1 := openAICacheWriteObservation{
		AccountID: 21, Model: "gpt-6-astra", CacheIdentity: "cache-oOO",
		InputTokens: 20000, CacheReadTokens: 15000,
		CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start,
	}
	turn2 := openAICacheWriteObservation{
		AccountID: 21, Model: "gpt-6-astra", CacheIdentity: "cache-oOO",
		InputTokens: 19000, CacheReadTokens: 16000,
		CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(time.Second),
	}
	turn3 := openAICacheWriteObservation{
		AccountID: 21, Model: "gpt-6-astra", CacheIdentity: "cache-oOO",
		InputTokens: 21000, CacheReadTokens: 17000,
		CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(2 * time.Second),
	}
	turn4 := openAICacheWriteObservation{
		AccountID: 21, Model: "gpt-6-astra", CacheIdentity: "cache-oOO",
		InputTokens: 22000, CacheReadTokens: 18000,
		CacheWriteState: openAICacheWriteFieldAbsent, ObservedAt: start.Add(3 * time.Second),
	}

	_, _, ok := tracker.observe(turn1, "req-1")
	require.False(t, ok)

	_, _, ok = tracker.observe(turn2, "req-2")
	require.False(t, ok, "shrinking total input must not infer or establish an eligible baseline")

	_, _, ok = tracker.observe(turn3, "req-3")
	require.False(t, ok, "first safe growth after an unsafe transition only re-establishes baseline")

	got, previousRequestID, ok := tracker.observe(turn4, "req-4")
	require.True(t, ok)
	require.Equal(t, "req-3", previousRequestID)
	require.Equal(t, 1000, got.Tokens)
}


func TestOpenAICacheWriteInferenceTracker_BoundsNewIdentities(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}
	for i := 0; i < maxOpenAICacheWriteInferenceEntries; i++ {
		_, _, ok := tracker.observe(openAICacheWriteObservation{
			AccountID: 1,
			Model: "gpt-6-astra",
			CacheIdentity: fmt.Sprintf("cache-%d", i),
			InputTokens: 1000,
			CacheReadTokens: 0,
			CacheWriteState: openAICacheWriteFieldAbsent,
			ObservedAt: start,
		}, fmt.Sprintf("req-%d", i))
		require.False(t, ok)
	}
	require.Len(t, tracker.entries, maxOpenAICacheWriteInferenceEntries)

	_, _, ok := tracker.observe(openAICacheWriteObservation{
		AccountID: 1,
		Model: "gpt-6-astra",
		CacheIdentity: "cache-overflow",
		InputTokens: 1000,
		CacheReadTokens: 0,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt: start.Add(time.Second),
	}, "req-overflow")
	require.False(t, ok)
	require.Len(t, tracker.entries, maxOpenAICacheWriteInferenceEntries)
	_, exists := tracker.entries[openAICacheWriteTrackerKey(1, "gpt-6-astra", "cache-overflow")]
	require.False(t, exists)
}
