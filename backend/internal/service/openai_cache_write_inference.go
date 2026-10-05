package service

import (
	"strings"
	"time"
)

const defaultOpenAICacheWriteInferenceWindow = 30 * time.Minute

type openAICacheWriteFieldState uint8

const (
	openAICacheWriteFieldAbsent openAICacheWriteFieldState = iota
	openAICacheWriteFieldExplicitZero
	openAICacheWriteFieldPositive
)

type openAICacheWriteObservation struct {
	AccountID        int64
	Model            string
	CacheIdentity    string
	Sequence         uint64
	InputTokens      int
	CacheReadTokens  int
	CacheWriteState  openAICacheWriteFieldState
	ObservedAt       time.Time
}

type openAICacheWriteInference struct {
	Tokens              int
	ResidualInputTokens int
	PreviousCacheRead   int
	NextCacheRead       int
	Source              string
}

func classifyOpenAICacheWriteField(present bool, tokens int) openAICacheWriteFieldState {
	if tokens > 0 {
		// A positive count is authoritative even when an intermediate typed
		// conversion did not preserve the separate presence bit.
		return openAICacheWriteFieldPositive
	}
	if present {
		return openAICacheWriteFieldExplicitZero
	}
	return openAICacheWriteFieldAbsent
}

// inferOpenAICacheWriteFromNextHit infers the previous turn's cache-write size
// from the next adjacent turn's cache-read growth.
//
// The inference is intentionally conservative:
//   - it only fills a missing upstream cache-write field;
//   - account/model/cache identity and sequence must match;
//   - the next cache-read must grow monotonically;
//   - the inferred write must fit inside the previous turn's uncached input;
//   - stale observations are rejected.
//
// The result is telemetry only. It must not be treated as authoritative billing
// data unless a separate policy explicitly opts into inferred values.
func inferOpenAICacheWriteFromNextHit(
	previous openAICacheWriteObservation,
	next openAICacheWriteObservation,
	maxGap time.Duration,
) (openAICacheWriteInference, bool) {
	if previous.CacheWriteState != openAICacheWriteFieldAbsent {
		return openAICacheWriteInference{}, false
	}
	if previous.AccountID <= 0 || previous.AccountID != next.AccountID {
		return openAICacheWriteInference{}, false
	}
	if strings.TrimSpace(previous.Model) == "" || strings.TrimSpace(previous.Model) != strings.TrimSpace(next.Model) {
		return openAICacheWriteInference{}, false
	}
	if strings.TrimSpace(previous.CacheIdentity) == "" ||
		strings.TrimSpace(previous.CacheIdentity) != strings.TrimSpace(next.CacheIdentity) {
		return openAICacheWriteInference{}, false
	}
	if previous.Sequence == 0 || next.Sequence != previous.Sequence+1 {
		return openAICacheWriteInference{}, false
	}
	if previous.InputTokens < 0 || previous.CacheReadTokens < 0 ||
		next.InputTokens < 0 || next.CacheReadTokens < 0 {
		return openAICacheWriteInference{}, false
	}
	if previous.InputTokens < previous.CacheReadTokens || next.InputTokens < next.CacheReadTokens {
		return openAICacheWriteInference{}, false
	}
	if next.InputTokens < previous.InputTokens {
		// A shrinking total usually indicates out-of-order completion, compaction,
		// or a different request shape. Skip rather than over-attribute a write.
		return openAICacheWriteInference{}, false
	}

	if maxGap <= 0 {
		maxGap = defaultOpenAICacheWriteInferenceWindow
	}
	if !previous.ObservedAt.IsZero() && !next.ObservedAt.IsZero() {
		if next.ObservedAt.Before(previous.ObservedAt) || next.ObservedAt.Sub(previous.ObservedAt) > maxGap {
			return openAICacheWriteInference{}, false
		}
	}

	cacheReadGrowth := next.CacheReadTokens - previous.CacheReadTokens
	if cacheReadGrowth <= 0 {
		// Equal cache-read does not prove zero cache-write: admission may be
		// delayed, evicted, or intentionally skipped.
		return openAICacheWriteInference{}, false
	}

	previousUncachedInput := previous.InputTokens - previous.CacheReadTokens
	if cacheReadGrowth > previousUncachedInput {
		// The next hit grew by more than the previous turn could have written.
		// This usually means the lineage is not adjacent or another cache
		// admission event was observed.
		return openAICacheWriteInference{}, false
	}

	return openAICacheWriteInference{
		Tokens:              cacheReadGrowth,
		ResidualInputTokens: previousUncachedInput - cacheReadGrowth,
		PreviousCacheRead:   previous.CacheReadTokens,
		NextCacheRead:       next.CacheReadTokens,
		Source:              "inferred_next_hit",
	}, true
}
