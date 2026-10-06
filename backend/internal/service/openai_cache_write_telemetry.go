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
	observationID  string
	eligibleForHit bool
}

type openAICacheWriteInferenceTracker struct {
	mu                sync.Mutex
	entries           map[string]openAICacheWriteTrackedObservation
	billingSnapshots  map[string]openAICacheWriteBillingSnapshot
	pendingInferences map[string]openAICacheWritePendingInference
	ready             map[string]*openAICacheWriteReadyReconciliation
	lastPrune         time.Time
	settingEpoch      uint64
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
	t.clearLocked()
	t.settingEpoch = epoch
}

func (t *openAICacheWriteInferenceTracker) clearAtSettingEpoch(epoch uint64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearLocked()
	t.settingEpoch = epoch
}

func (t *openAICacheWriteInferenceTracker) clearLocked() {
	t.entries = nil
	t.billingSnapshots = nil
	t.pendingInferences = nil
	t.ready = nil
	t.lastPrune = time.Time{}
}

func (t *openAICacheWriteInferenceTracker) observe(
	observation openAICacheWriteObservation,
	observationID string,
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
		// Inference/billing metadata must never create unbounded process memory.
		return openAICacheWriteInference{}, "", false
	}

	previous, ok := t.entries[key]
	if !ok || previous.observation.ObservedAt.IsZero() ||
		observation.ObservedAt.Before(previous.observation.ObservedAt) ||
		observation.ObservedAt.Sub(previous.observation.ObservedAt) > defaultOpenAICacheWriteInferenceWindow {
		observation.Sequence = 1
		t.entries[key] = openAICacheWriteTrackedObservation{
			observation:    observation,
			observationID:  observationID,
			eligibleForHit: observation.CacheWriteState == openAICacheWriteFieldAbsent,
		}
		return openAICacheWriteInference{}, "", false
	}

	if observationID != "" && observationID == previous.observationID {
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
		observationID:  observationID,
		eligibleForHit: currentEligible,
	}
	return inference, previous.observationID, inferred
}

func (t *openAICacheWriteInferenceTracker) registerInference(
	observationID string,
	inference openAICacheWriteInference,
	observedAt time.Time,
) {
	if t == nil || strings.TrimSpace(observationID) == "" || inference.Tokens <= 0 {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(observedAt)

	if t.ready == nil {
		t.ready = make(map[string]*openAICacheWriteReadyReconciliation)
	}
	if _, exists := t.ready[observationID]; exists {
		return
	}
	if snapshot, ok := t.billingSnapshots[observationID]; ok {
		delete(t.billingSnapshots, observationID)
		delete(t.pendingInferences, observationID)
		t.ready[observationID] = &openAICacheWriteReadyReconciliation{
			ObservationID: observationID,
			Snapshot:      snapshot,
			Inference:     inference,
			ObservedAt:    observedAt,
		}
		return
	}
	if t.pendingInferences == nil {
		t.pendingInferences = make(map[string]openAICacheWritePendingInference)
	}
	t.pendingInferences[observationID] = openAICacheWritePendingInference{
		Inference:  inference,
		ObservedAt: observedAt,
	}
}

func (t *openAICacheWriteInferenceTracker) attachBillingSnapshot(
	observationID string,
	snapshot openAICacheWriteBillingSnapshot,
) {
	if t == nil || strings.TrimSpace(observationID) == "" {
		return
	}
	if snapshot.ObservedAt.IsZero() {
		snapshot.ObservedAt = time.Now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(snapshot.ObservedAt)

	if t.ready != nil {
		if _, exists := t.ready[observationID]; exists {
			return
		}
	}
	if pending, ok := t.pendingInferences[observationID]; ok {
		if t.ready == nil {
			t.ready = make(map[string]*openAICacheWriteReadyReconciliation)
		}
		delete(t.pendingInferences, observationID)
		delete(t.billingSnapshots, observationID)
		t.ready[observationID] = &openAICacheWriteReadyReconciliation{
			ObservationID: observationID,
			Snapshot:      snapshot,
			Inference:     pending.Inference,
			ObservedAt:    pending.ObservedAt,
		}
		return
	}
	if t.billingSnapshots == nil {
		t.billingSnapshots = make(map[string]openAICacheWriteBillingSnapshot)
	}
	if len(t.billingSnapshots) >= maxOpenAICacheWriteInferenceEntries {
		return
	}
	t.billingSnapshots[observationID] = snapshot
}

func (t *openAICacheWriteInferenceTracker) claimReadyReconciliation() (string, *openAICacheWriteReadyReconciliation) {
	if t == nil {
		return "", nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, ready := range t.ready {
		if ready == nil || ready.inFlight {
			continue
		}
		ready.inFlight = true
		copyReady := *ready
		return key, &copyReady
	}
	return "", nil
}

func (t *openAICacheWriteInferenceTracker) finishReadyReconciliation(key string, success bool) {
	if t == nil || key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ready := t.ready[key]
	if ready == nil {
		return
	}
	if success {
		delete(t.ready, key)
		delete(t.billingSnapshots, key)
		delete(t.pendingInferences, key)
		return
	}
	ready.inFlight = false
}

func (t *openAICacheWriteInferenceTracker) pruneLocked(now time.Time) {
	if t == nil {
		return
	}
	pruneInterval := 5 * time.Minute
	if len(t.entries) >= maxOpenAICacheWriteInferenceEntries ||
		len(t.billingSnapshots) >= maxOpenAICacheWriteInferenceEntries {
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
	for key, snapshot := range t.billingSnapshots {
		if snapshot.ObservedAt.Before(cutoff) {
			delete(t.billingSnapshots, key)
		}
	}
	for key, pending := range t.pendingInferences {
		if pending.ObservedAt.Before(cutoff) {
			delete(t.pendingInferences, key)
		}
	}
	for key, ready := range t.ready {
		if ready == nil || ready.ObservedAt.Before(cutoff) {
			delete(t.ready, key)
		}
	}
	t.lastPrune = now
}

// ObserveOpenAICacheWriteTelemetry records one successful OpenAI OAuth turn,
// infers the previous missing cache-write counter when the next rolling hit
// proves it, and registers that inference for post-hoc billing reconciliation.
//
// The returned observation ID must be carried into OpenAIRecordUsageInput so the
// original turn's exact billing snapshot can later be paired with the inference.
func (s *OpenAIGatewayService) ObserveOpenAICacheWriteTelemetry(
	ctx context.Context,
	account *Account,
	requestedModel string,
	cacheIdentity string,
	result *OpenAIForwardResult,
) string {
	if s == nil || account == nil || result == nil || !account.IsOpenAIOAuthLike() {
		return ""
	}
	enabled := s.settingService != nil && s.settingService.IsOpenAICacheWriteInferenceEnabled(ctx)
	epoch := openAICacheWriteInferenceSettingEpoch.Load()
	if !enabled {
		s.openaiCacheWriteInferenceTracker.clearAtSettingEpoch(epoch)
		return ""
	}
	s.openaiCacheWriteInferenceTracker.resetForSettingEpoch(epoch)
	cacheIdentity = strings.TrimSpace(cacheIdentity)
	if cacheIdentity == "" || result.Usage.InputTokens <= 0 {
		return ""
	}

	model := strings.TrimSpace(result.UpstreamModel)
	if model == "" {
		model = strings.TrimSpace(result.Model)
	}
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	if model == "" {
		return ""
	}

	observation := openAICacheWriteObservation{
		AccountID:       account.ID,
		Model:           model,
		CacheIdentity:   cacheIdentity,
		InputTokens:     result.Usage.InputTokens,
		CacheReadTokens: result.Usage.CacheReadInputTokens,
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
	if observationID == "" {
		observationID = "cacheobs:" + generateRequestID()
	}

	inference, previousObservationID, ok := s.openaiCacheWriteInferenceTracker.observe(observation, observationID)
	if !ok {
		return observationID
	}

	s.openaiCacheWriteInferenceTracker.registerInference(previousObservationID, inference, time.Now())
	logger.L().With(
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", account.ID),
		zap.String("model", model),
		zap.String("cache_identity_sha256", hashSensitiveValueForLog(cacheIdentity)),
		zap.String("previous_observation_id_sha256", hashSensitiveValueForLog(previousObservationID)),
		zap.String("current_observation_id_sha256", hashSensitiveValueForLog(observationID)),
		zap.String("cache_write_source", inference.Source),
		zap.Int("inferred_cache_write_tokens", inference.Tokens),
		zap.Int("inferred_uncached_input_tokens", inference.ResidualInputTokens),
		zap.Int("previous_cache_read_tokens", inference.PreviousCacheRead),
		zap.Int("current_cache_read_tokens", inference.NextCacheRead),
	).Debug("openai.cache_write_inferred")

	return observationID
}
