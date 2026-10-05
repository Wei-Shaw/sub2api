package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const maxOpenAICacheWriteInferenceEntries = 8192

type openAICacheWriteTrackedObservation struct {
	observation    openAICacheWriteObservation
	requestID      string
	eligibleForHit bool
}

type openAICacheWriteInferenceTracker struct {
	mu           sync.Mutex
	entries      map[string]openAICacheWriteTrackedObservation
	lastPrune    time.Time
	settingEpoch uint64
}

func openAICacheWriteTrackerKey(accountID int64, model, cacheIdentity string) string {
	return strconv.FormatInt(accountID, 10) + "\x00" + strings.TrimSpace(model) + "\x00" + strings.TrimSpace(cacheIdentity)
}

func (t *openAICacheWriteInferenceTracker) resetForSettingEpoch(epoch uint64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.settingEpoch == epoch {
		return
	}
	t.entries = nil
	t.lastPrune = time.Time{}
	t.settingEpoch = epoch
}

func (t *openAICacheWriteInferenceTracker) clearAtSettingEpoch(epoch uint64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = nil
	t.lastPrune = time.Time{}
	t.settingEpoch = epoch
}

func (t *openAICacheWriteInferenceTracker) observe(
	observation openAICacheWriteObservation,
	requestID string,
) (openAICacheWriteInference, string, bool) {
	if t == nil || observation.AccountID <= 0 || strings.TrimSpace(observation.Model) == "" ||
		strings.TrimSpace(observation.CacheIdentity) == "" {
		return openAICacheWriteInference{}, "", false
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now()
	}

	key := openAICacheWriteTrackerKey(observation.AccountID, observation.Model, observation.CacheIdentity)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.entries == nil {
		t.entries = make(map[string]openAICacheWriteTrackedObservation)
	}
	t.pruneLocked(observation.ObservedAt)

	if _, exists := t.entries[key]; !exists && len(t.entries) >= maxOpenAICacheWriteInferenceEntries {
		// Telemetry must never create unbounded process memory. Keep existing
		// lineages warm and admit new ones after stale entries are pruned.
		return openAICacheWriteInference{}, "", false
	}

	previous, ok := t.entries[key]
	if !ok || previous.observation.ObservedAt.IsZero() ||
		observation.ObservedAt.Before(previous.observation.ObservedAt) ||
		observation.ObservedAt.Sub(previous.observation.ObservedAt) > defaultOpenAICacheWriteInferenceWindow {
		observation.Sequence = 1
		t.entries[key] = openAICacheWriteTrackedObservation{
			observation:    observation,
			requestID:      requestID,
			eligibleForHit: observation.CacheWriteState == openAICacheWriteFieldAbsent,
		}
		return openAICacheWriteInference{}, "", false
	}

	if requestID != "" && requestID == previous.requestID {
		// The same completed request may be observed more than once by fallback
		// paths. It is not a new turn and must not advance the lineage.
		return openAICacheWriteInference{}, "", false
	}

	observation.Sequence = previous.observation.Sequence + 1
	cacheReadGrew := observation.CacheReadTokens > previous.observation.CacheReadTokens
	monotonicInput := observation.InputTokens >= previous.observation.InputTokens
	validTotals := previous.observation.InputTokens >= previous.observation.CacheReadTokens &&
		observation.InputTokens >= observation.CacheReadTokens
	inference := openAICacheWriteInference{}
	inferred := false
	if previous.eligibleForHit && cacheReadGrew && monotonicInput && validTotals {
		inference, inferred = inferOpenAICacheWriteFromNextHit(
			previous.observation,
			observation,
			defaultOpenAICacheWriteInferenceWindow,
		)
	}

	// A non-growing or structurally invalid turn makes later admission ambiguous:
	// an older write may be admitted asynchronously, or concurrent turns may
	// have completed out of order. The first later safe growth only establishes
	// a new baseline; inference resumes on the following rolling hit.
	currentEligible := observation.CacheWriteState == openAICacheWriteFieldAbsent &&
		cacheReadGrew && monotonicInput && validTotals
	t.entries[key] = openAICacheWriteTrackedObservation{
		observation:    observation,
		requestID:      requestID,
		eligibleForHit: currentEligible,
	}
	return inference, previous.requestID, inferred
}

func (t *openAICacheWriteInferenceTracker) pruneLocked(now time.Time) {
	if t == nil || len(t.entries) == 0 {
		return
	}
	pruneInterval := 5 * time.Minute
	if len(t.entries) >= maxOpenAICacheWriteInferenceEntries {
		pruneInterval = time.Minute
	}
	if !t.lastPrune.IsZero() && now.Sub(t.lastPrune) < pruneInterval {
		return
	}
	cutoff := now.Add(-defaultOpenAICacheWriteInferenceWindow)
	for key, entry := range t.entries {
		if entry.observation.ObservedAt.Before(cutoff) {
			delete(t.entries, key)
		}
	}
	t.lastPrune = now
}

// ObserveOpenAICacheWriteTelemetry records cache usage for an OpenAI OAuth turn
// and emits a debug diagnostic when the next adjacent cache hit can safely
// infer the previous turn's missing cache-write counter.
//
// Inferred values are deliberately not written back into billing usage.
func (s *OpenAIGatewayService) ObserveOpenAICacheWriteTelemetry(
	ctx context.Context,
	account *Account,
	requestedModel string,
	cacheIdentity string,
	result *OpenAIForwardResult,
) {
	if s == nil || account == nil || result == nil || !account.IsOpenAIOAuthLike() {
		return
	}
	enabled := s.settingService != nil && s.settingService.IsOpenAICacheWriteInferenceEnabled(ctx)
	epoch := openAICacheWriteInferenceSettingEpoch.Load()
	if !enabled {
		s.openaiCacheWriteInferenceTracker.clearAtSettingEpoch(epoch)
		return
	}
	s.openaiCacheWriteInferenceTracker.resetForSettingEpoch(epoch)
	cacheIdentity = strings.TrimSpace(cacheIdentity)
	if cacheIdentity == "" || result.Usage.InputTokens <= 0 {
		return
	}

	model := strings.TrimSpace(result.UpstreamModel)
	if model == "" {
		model = strings.TrimSpace(result.Model)
	}
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	if model == "" {
		return
	}

	observation := openAICacheWriteObservation{
		AccountID:        account.ID,
		Model:            model,
		CacheIdentity:    cacheIdentity,
		InputTokens:      result.Usage.InputTokens,
		CacheReadTokens:  result.Usage.CacheReadInputTokens,
		CacheWriteState: classifyOpenAICacheWriteField(
			result.Usage.CacheCreationInputTokensPresent,
			result.Usage.CacheCreationInputTokens,
		),
		ObservedAt: time.Now(),
	}

	observationID := strings.TrimSpace(result.RequestID)
	if observationID == "" {
		observationID = strings.TrimSpace(result.ResponseID)
	}
	inference, previousRequestID, ok := s.openaiCacheWriteInferenceTracker.observe(observation, observationID)
	if !ok {
		return
	}

	logger.L().With(
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", account.ID),
		zap.String("model", model),
		zap.String("cache_identity_sha256", hashSensitiveValueForLog(cacheIdentity)),
		zap.String("previous_request_id_sha256", hashSensitiveValueForLog(previousRequestID)),
		zap.String("current_request_id_sha256", hashSensitiveValueForLog(observationID)),
		zap.String("cache_write_source", inference.Source),
		zap.Int("inferred_cache_write_tokens", inference.Tokens),
		zap.Int("inferred_uncached_input_tokens", inference.ResidualInputTokens),
		zap.Int("previous_cache_read_tokens", inference.PreviousCacheRead),
		zap.Int("current_cache_read_tokens", inference.NextCacheRead),
	).Debug("openai.cache_write_inferred")
}
