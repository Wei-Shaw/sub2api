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
	_, _, ok := observeCacheWriteTrackerFixture(tracker, first, "req-1")
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
	got, previousRequestID, ok := observeCacheWriteTrackerFixture(tracker, second, "req-2")
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
	observeCacheWriteTrackerFixture(tracker, first, "same-request")

	duplicate := first
	duplicate.CacheReadTokens = 9000
	duplicate.ObservedAt = start.Add(time.Second)
	_, _, ok := observeCacheWriteTrackerFixture(tracker, duplicate, "same-request")
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
	got, _, ok := observeCacheWriteTrackerFixture(tracker, next, "req-next")
	require.True(t, ok)
	require.Equal(t, 1000, got.Tokens)
	require.Equal(t, 1000, got.ResidualInputTokens)
}

func TestOpenAICacheWriteInferenceTracker_StaleTurnResetsLineage(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}
	observeCacheWriteTrackerFixture(tracker, openAICacheWriteObservation{
		AccountID:       9,
		Model:           "gpt-6-sol",
		CacheIdentity:   "cache-c",
		InputTokens:     10000,
		CacheReadTokens: 5000,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start,
	}, "req-old")

	_, _, ok := observeCacheWriteTrackerFixture(tracker, openAICacheWriteObservation{
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

	_, _, ok := observeCacheWriteTrackerFixture(tracker, turns[0], "req-1")
	require.False(t, ok)

	_, _, ok = observeCacheWriteTrackerFixture(tracker, turns[1], "req-2")
	require.False(t, ok, "a miss cannot prove a zero write")

	_, _, ok = observeCacheWriteTrackerFixture(tracker, turns[2], "req-3")
	require.False(t, ok, "first hit after a miss is ambiguous and only rebaselines")

	got, previousRequestID, ok := observeCacheWriteTrackerFixture(tracker, turns[3], "req-4")
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

	_, _, ok := observeCacheWriteTrackerFixture(tracker, turn1, "req-1")
	require.False(t, ok)

	_, _, ok = observeCacheWriteTrackerFixture(tracker, turn2, "req-2")
	require.False(t, ok, "shrinking total input must not infer or establish an eligible baseline")

	_, _, ok = observeCacheWriteTrackerFixture(tracker, turn3, "req-3")
	require.False(t, ok, "first safe growth after an unsafe transition only re-establishes baseline")

	got, previousRequestID, ok := observeCacheWriteTrackerFixture(tracker, turn4, "req-4")
	require.True(t, ok)
	require.Equal(t, "req-3", previousRequestID)
	require.Equal(t, 1000, got.Tokens)
}

func TestOpenAICacheWriteInferenceTracker_BoundsNewIdentities(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)
	tracker := &openAICacheWriteInferenceTracker{}
	for i := 0; i < maxOpenAICacheWriteInferenceEntries; i++ {
		_, _, ok := observeCacheWriteTrackerFixture(tracker, openAICacheWriteObservation{
			AccountID:       1,
			Model:           "gpt-6-astra",
			CacheIdentity:   fmt.Sprintf("cache-%d", i),
			InputTokens:     1000,
			CacheReadTokens: 0,
			CacheWriteState: openAICacheWriteFieldAbsent,
			ObservedAt:      start,
		}, fmt.Sprintf("req-%d", i))
		require.False(t, ok)
	}
	require.Len(t, tracker.entries, maxOpenAICacheWriteInferenceEntries)

	_, _, ok := observeCacheWriteTrackerFixture(tracker, openAICacheWriteObservation{
		AccountID:       1,
		Model:           "gpt-6-astra",
		CacheIdentity:   "cache-overflow",
		InputTokens:     1000,
		CacheReadTokens: 0,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      start.Add(time.Second),
	}, "req-overflow")
	require.False(t, ok)
	require.Len(t, tracker.entries, maxOpenAICacheWriteInferenceEntries)
	_, exists := tracker.entries[openAICacheWriteTrackerKey(1, "gpt-6-astra", "cache-overflow")]
	require.False(t, exists)
}

// Legacy arithmetic/expiry tests explicitly provide valid local admission and
// identical-prompt evidence. Missing-evidence behavior is covered separately.
func observeCacheWriteTrackerFixture(tracker *openAICacheWriteInferenceTracker, observation openAICacheWriteObservation, id string) (openAICacheWriteInference, string, bool) {
	observation.APIKeyID = 1
	observation.admitted = true
	observation.StartedAt = observation.ObservedAt
	observation.Prompt = buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"fixture prompt"}]}`))
	observation.scopeKey = openAICacheWriteTrackerKey(observation.AccountID, observation.Model, observation.CacheIdentity)
	tracker.mu.Lock()
	prev, ok := tracker.entries[observation.scopeKey]
	tracker.mu.Unlock()
	if ok {
		observation.PredecessorID = prev.observationID
		observation.Prompt = cacheWriteFixtureSuccessorPrompt(prev.observation, observation, id)
	}
	observation.Output = openAICacheWriteOutputEvidence{Valid: true}
	return tracker.observe(observation, id)
}

// Counter-only unit fixtures model full-history growth as a single direct user
// item after an empty, completed output; production receives hashes from wire JSON.
func cacheWriteFixtureSuccessorPrompt(previous, next openAICacheWriteObservation, id string) openAICacheWritePromptEvidence {
	if previous.InputTokens == next.InputTokens {
		return previous.Prompt
	}
	prompt := previous.Prompt
	prompt.InputHashes = append([]string(nil), previous.Prompt.InputHashes...)
	prompt.items = append([]openAICacheWriteItemEvidence(nil), previous.Prompt.items...)
	item := buildOpenAICacheWritePromptEvidence([]byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"role":"user","content":%q}]}`, id)))
	prompt.InputHashes = append(prompt.InputHashes, item.InputHashes[0])
	prompt.items = append(prompt.items, item.items[0])
	return prompt
}
