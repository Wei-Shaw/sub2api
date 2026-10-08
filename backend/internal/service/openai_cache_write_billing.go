package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const openAICacheWriteBillingEpsilon = 1e-12

// OpenAICacheWriteUsageCorrection is an absolute, idempotent rewrite of the
// original usage-log row after a later cache hit proves that part of the
// previous ordinary-input bucket was actually cache creation.
type OpenAICacheWriteUsageCorrection struct {
	RequestID string
	APIKeyID  int64

	OriginalInputTokens         int
	OriginalCacheCreationTokens int

	CorrectedInputTokens         int
	CorrectedCacheCreationTokens int

	CorrectedInputCost         float64
	CorrectedCacheCreationCost float64
	CorrectedTotalCost         float64
	CorrectedActualCost        float64
	CorrectedAccountStatsCost  *float64
}

type openAICacheWriteBillingSnapshot struct {
	Epoch            uint64
	OriginalUsageLog *UsageLog
	ObservedAt       time.Time
	ObservationID    string
	RequestID        string
	Model            string
	BillingType      int8
	ServiceTier      *string
	ReasoningEffort  *string

	User          *User
	APIKey        *APIKey
	Account       *Account
	Subscription  *UserSubscription
	APIKeyService APIKeyQuotaUpdater
	Platform      string

	IsSubscriptionBill         bool
	SimpleModeKeyRateLimitOnly bool
	AccountRateMultiplier      float64
	RequestPayloadHash         string

	OriginalInputTokens         int
	OriginalCacheCreationTokens int
	OriginalInputCost           float64
	OriginalCacheCreationCost   float64
	OriginalTotalCost           float64
	OriginalActualCost          float64
	OriginalAccountStatsCost    *float64

	DeltaInputCostPerToken         float64
	DeltaCacheCreationCostPerToken float64
	DeltaTotalCostPerToken         float64
	DeltaActualCostPerToken        float64
	DeltaAccountStatsCostPerToken  *float64
}

type openAICacheWritePendingInference struct {
	Inference  openAICacheWriteInference
	ObservedAt time.Time
}

type openAICacheWriteReadyReconciliation struct {
	ObservationID string
	Snapshot      openAICacheWriteBillingSnapshot
	Inference     openAICacheWriteInference
	ObservedAt    time.Time
	inFlight      bool
}

func openAICacheWriteAdjustmentRequestID(requestID string, apiKeyID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("cache-write-reconcile|%d|%s", apiKeyID, strings.TrimSpace(requestID))))
	return "cwr:" + hex.EncodeToString(sum[:20])
}

func cloneOptionalString(v *string) *string {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneOptionalFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

// openAICacheWritePricingPlan contains only the pricing selected while computing
// the original bill. No calculator in it performs a resolver or settings read.
type openAICacheWritePricingPlan struct {
	calculate        func(UsageTokens) *CostBreakdown
	standard         *openAICacheWritePricingPlan
	requiresStandard bool
	accountStats     func(UsageTokens, float64) *float64
}

func newOpenAICacheWriteTokenPricingPlan(
	billing *BillingService, pricing *ModelPricing, multiplier float64,
	serviceTier string, longContext bool, timeMultiplier, effortMultiplier float64,
) *openAICacheWritePricingPlan {
	frozen := *pricing
	frozen.FastMultiplier = cloneOptionalFloat64(pricing.FastMultiplier)
	frozen.FlexMultiplier = cloneOptionalFloat64(pricing.FlexMultiplier)
	frozen.ReasoningEffortMultipliers = maps.Clone(pricing.ReasoningEffortMultipliers)
	return &openAICacheWritePricingPlan{calculate: func(tokens UsageTokens) *CostBreakdown {
		cost := billing.computeTokenBreakdown(&frozen, tokens, multiplier, serviceTier, longContext)
		applyCostBreakdownMultiplier(cost, timeMultiplier)
		applyCostBreakdownMultiplier(cost, effortMultiplier)
		cost.BillingMode = string(BillingModeToken)
		return cost
	}}
}

// Resolve the same account-stat priority chain as applyAccountStatsCost, once.
// The selected rule/interval or catalog plan then serves both original and probe.
func (s *OpenAIGatewayService) applyFrozenOpenAICacheWriteAccountStatsCost(
	ctx context.Context, usageLog *UsageLog, cost *CostBreakdown, tokens UsageTokens,
	upstreamModel, requestedModel string, pricingAt time.Time, longContext bool,
) {
	if usageLog == nil || usageLog.GroupID == nil || s.channelService == nil || cost.cacheWritePricing == nil {
		return
	}
	model := upstreamModel
	if model == "" {
		model = requestedModel
	}
	if model == "" {
		return
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, *usageLog.GroupID)
	if err != nil || channel == nil {
		return
	}
	platform := s.channelService.GetGroupPlatform(ctx, *usageLog.GroupID)
	effort := optionalStringValue(usageLog.ReasoningEffort)
	requestCount := max(usageLog.ImageCount, 1)
	for _, rule := range channel.AccountStatsPricingRules {
		if !matchAccountStatsRule(&rule, usageLog.AccountID, *usageLog.GroupID) {
			continue
		}
		pricing := findPricingForModel(rule.Pricing, platform, strings.ToLower(model))
		if pricing == nil {
			continue
		}
		frozen := *pricing
		effortMultiplier := reasoningEffortBillingMultiplier(effort, pricing.ReasoningEffortMultipliers)
		if pricing.BillingMode != BillingModePerRequest && pricing.BillingMode != BillingModeImage && pricing.BillingMode != BillingModeVideo {
			if iv := FindMatchingInterval(pricing.Intervals, tokens.InputTokens+tokens.OutputTokens+tokens.CacheCreationTokens+tokens.CacheReadTokens); iv != nil {
				frozen = ChannelModelPricing{InputPrice: iv.InputPrice, OutputPrice: iv.OutputPrice,
					CacheWritePrice: iv.CacheWritePrice, CacheWrite1hPrice: iv.CacheWrite1hPrice,
					CacheReadPrice: iv.CacheReadPrice, PerRequestPrice: iv.PerRequestPrice}
			}
		}
		// The one-token move preserves context size, so freeze its selected
		// interval and the scalar prices instead of retaining a mutable card.
		frozen.Intervals = nil
		frozen.InputPrice = cloneOptionalFloat64(frozen.InputPrice)
		frozen.OutputPrice = cloneOptionalFloat64(frozen.OutputPrice)
		frozen.CacheWritePrice = cloneOptionalFloat64(frozen.CacheWritePrice)
		frozen.CacheWrite1hPrice = cloneOptionalFloat64(frozen.CacheWrite1hPrice)
		frozen.CacheReadPrice = cloneOptionalFloat64(frozen.CacheReadPrice)
		frozen.ImageOutputPrice = cloneOptionalFloat64(frozen.ImageOutputPrice)
		frozen.PerRequestPrice = cloneOptionalFloat64(frozen.PerRequestPrice)
		calculate := func(usage UsageTokens, _ float64) *float64 {
			value := calculateStatsCost(&frozen, usage, requestCount)
			if value != nil {
				*value *= effortMultiplier
			}
			return value
		}
		if value := calculate(tokens, cost.TotalCost); value != nil {
			usageLog.AccountStatsCost = value
			cost.cacheWritePricing.accountStats = calculate
			return
		}
		// Preserve tryCustomRules: the first matching card owns the outcome;
		// a nil cost falls through to channel/catalog policy, not a later rule.
		break
	}
	if channel.ApplyPricingToAccountStats {
		if cost.TotalCost > 0 {
			usageLog.AccountStatsCost = cloneOptionalFloat64(&cost.TotalCost)
			cost.cacheWritePricing.accountStats = func(_ UsageTokens, total float64) *float64 { return &total }
		}
		return
	}
	if s.billingService == nil {
		return
	}
	resolver := NewModelPricingResolver(nil, s.billingService)
	resolved := resolver.Resolve(ctx, PricingInput{Model: model})
	resolved.longContextPricingEnabled = longContext
	statsCost, err := s.billingService.CalculateCostUnified(CostInput{
		Ctx: ctx, Model: model, Tokens: tokens, RateMultiplier: 1,
		ServiceTier:     normalizeBillingServiceTier(optionalStringValue(usageLog.ServiceTier)),
		ReasoningEffort: effort, PricingAt: pricingAt, Resolver: resolver, Resolved: resolved,
		captureCacheWritePricing: true,
	})
	if err != nil || statsCost == nil || statsCost.TotalCost <= 0 || statsCost.cacheWritePricing == nil {
		return
	}
	usageLog.AccountStatsCost = cloneOptionalFloat64(&statsCost.TotalCost)
	plan := statsCost.cacheWritePricing
	cost.cacheWritePricing.accountStats = func(usage UsageTokens, _ float64) *float64 {
		value := plan.calculate(usage).TotalCost
		return &value
	}
}

// Keep a private immutable copy for recovery if the charge commits but the
// usage-log insert fails. Relations are not persisted by Create and are omitted.
func cloneOpenAICacheWriteUsageLog(log *UsageLog) *UsageLog {
	if log == nil {
		return nil
	}
	cloned := *log
	cloned.UpstreamModel = cloneOptionalString(log.UpstreamModel)
	cloned.UpstreamResponseModel = cloneOptionalString(log.UpstreamResponseModel)
	cloned.ModelMappingChain = cloneOptionalString(log.ModelMappingChain)
	cloned.BillingTier = cloneOptionalString(log.BillingTier)
	cloned.BillingMode = cloneOptionalString(log.BillingMode)
	cloned.ServiceTier = cloneOptionalString(log.ServiceTier)
	cloned.ReasoningEffort = cloneOptionalString(log.ReasoningEffort)
	cloned.RequestedReasoningEffort = cloneOptionalString(log.RequestedReasoningEffort)
	cloned.InboundEndpoint = cloneOptionalString(log.InboundEndpoint)
	cloned.UpstreamEndpoint = cloneOptionalString(log.UpstreamEndpoint)
	cloned.UserAgent = cloneOptionalString(log.UserAgent)
	cloned.IPAddress = cloneOptionalString(log.IPAddress)
	cloned.SessionID = cloneOptionalString(log.SessionID)
	cloned.UpstreamRequestID = cloneOptionalString(log.UpstreamRequestID)
	cloned.ImageSize = cloneOptionalString(log.ImageSize)
	cloned.ImageInputSize = cloneOptionalString(log.ImageInputSize)
	cloned.ImageOutputSize = cloneOptionalString(log.ImageOutputSize)
	cloned.ImageSizeSource = cloneOptionalString(log.ImageSizeSource)
	cloned.MediaType = cloneOptionalString(log.MediaType)
	cloned.VideoResolution = cloneOptionalString(log.VideoResolution)
	cloned.AccountRateMultiplier = cloneOptionalFloat64(log.AccountRateMultiplier)
	cloned.AccountStatsCost = cloneOptionalFloat64(log.AccountStatsCost)
	if log.UpstreamModelMismatch != nil {
		value := *log.UpstreamModelMismatch
		cloned.UpstreamModelMismatch = &value
	}
	if log.ChannelID != nil {
		value := *log.ChannelID
		cloned.ChannelID = &value
	}
	if log.GroupID != nil {
		value := *log.GroupID
		cloned.GroupID = &value
	}
	if log.SubscriptionID != nil {
		value := *log.SubscriptionID
		cloned.SubscriptionID = &value
	}
	if log.DurationMs != nil {
		value := *log.DurationMs
		cloned.DurationMs = &value
	}
	if log.FirstTokenMs != nil {
		value := *log.FirstTokenMs
		cloned.FirstTokenMs = &value
	}
	if log.VideoDurationSeconds != nil {
		value := *log.VideoDurationSeconds
		cloned.VideoDurationSeconds = &value
	}
	cloned.ImageSizeBreakdown = maps.Clone(log.ImageSizeBreakdown)
	cloned.User, cloned.APIKey, cloned.Account, cloned.Group, cloned.Subscription = nil, nil, nil, nil, nil
	return &cloned
}

func (t *openAICacheWriteInferenceTracker) lookupBillingSnapshot(observationID string) *openAICacheWriteBillingSnapshot {
	if t == nil || observationID == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if snapshot, ok := t.billingSnapshots[observationID]; ok {
		return &snapshot
	}
	if ready := t.ready[observationID]; ready != nil {
		snapshot := ready.Snapshot
		return &snapshot
	}
	return nil
}

func (s *OpenAIGatewayService) createOpenAICacheWriteUsageLog(ctx context.Context, usageLog *UsageLog) (bool, error) {
	if s.usageLogRepo == nil {
		return false, errors.New("usage log repository is required for cache write billing")
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		// Each retry gets its own detached deadline. The charged usage row must
		// survive worker cancellation and exhaustion of the first write window.
		usageCtx, cancel := detachedBillingContext(ctx)
		var inserted bool
		inserted, err = s.usageLogRepo.Create(usageCtx, cloneOpenAICacheWriteUsageLog(usageLog))
		cancel()
		if err == nil {
			// inserted=false is the successful idempotent recovery case.
			return inserted, nil
		}
	}
	return false, err
}

func (s *OpenAIGatewayService) openAICacheWriteBillingEnabled(ctx context.Context, epoch uint64) bool {
	if s == nil || s.settingService == nil || openAICacheWriteInferenceSettingEpoch.Load() != epoch {
		return false
	}
	return s.settingService.IsOpenAICacheWriteInferenceEnabled(ctx) &&
		openAICacheWriteInferenceSettingEpoch.Load() == epoch
}

// prepareOpenAICacheWriteBillingSnapshot calculates the exact marginal price of
// moving one token from ordinary input into cache creation, using the already
// selected billing model, service tier, rate multiplier, long-context policy,
// and original request pricing timestamp.
//
// The move keeps total context unchanged, so all threshold/tier decisions remain
// on the same side of their boundary. Later reconciliation can multiply this
// marginal delta by the inferred write-token count without re-reading mutable
// pricing state.
func (s *OpenAIGatewayService) prepareOpenAICacheWriteBillingSnapshot(
	ctx context.Context,
	input *OpenAIRecordUsageInput,
	usageLog *UsageLog,
	cost *CostBreakdown,
	tokens UsageTokens,
	isSubscriptionBilling bool,
	accountRateMultiplier float64,
	simpleModeKeyRateLimitOnly bool,
) *openAICacheWriteBillingSnapshot {
	if s == nil || input == nil || input.Result == nil || usageLog == nil || cost == nil ||
		strings.TrimSpace(input.CacheWriteObservationID) == "" ||
		!s.openAICacheWriteBillingEnabled(ctx, input.CacheWriteSettingEpoch) {
		return nil
	}
	result := input.Result
	if result.Usage.CacheCreationInputTokensPresent || result.Usage.CacheCreationInputTokens != 0 ||
		tokens.InputTokens <= 0 {
		return nil
	}
	// Cache-write inference is intended for text prompt-cache accounting. Media,
	// search, and audio pricing have independent units/surcharges and must not be
	// retroactively rewritten by this path.
	if result.ImageCount > 0 || result.VideoCount > 0 || result.SearchCount > 0 ||
		result.WebSearchCalls > 0 || result.AudioUsage != nil ||
		tokens.ImageInputTokens > 0 || tokens.ImageOutputTokens > 0 || tokens.ImageCacheReadTokens > 0 {
		return nil
	}
	if usageLog.BillingMode == nil || *usageLog.BillingMode != string(BillingModeToken) {
		return nil
	}

	probeTokens := tokens
	probeTokens.InputTokens--
	probeTokens.CacheCreationTokens++

	plan := cost.cacheWritePricing
	if plan == nil || plan.calculate == nil {
		return nil
	}
	probeCost := plan.calculate(probeTokens)
	if plan.requiresStandard {
		if plan.standard == nil || plan.standard.calculate == nil {
			return nil
		}
		probeCost.ActualCost = plan.standard.calculate(probeTokens).ActualCost
	}

	var deltaAccountStatsCost *float64
	if usageLog.AccountStatsCost != nil {
		if plan.accountStats == nil {
			return nil
		}
		probeStats := plan.accountStats(probeTokens, probeCost.TotalCost)
		if probeStats == nil {
			return nil
		}
		delta := *probeStats - *usageLog.AccountStatsCost
		deltaAccountStatsCost = &delta
	}

	return &openAICacheWriteBillingSnapshot{
		Epoch:                          input.CacheWriteSettingEpoch,
		OriginalUsageLog:               cloneOpenAICacheWriteUsageLog(usageLog),
		ObservedAt:                     time.Now(),
		ObservationID:                  strings.TrimSpace(input.CacheWriteObservationID),
		RequestID:                      usageLog.RequestID,
		Model:                          usageLog.Model,
		BillingType:                    usageLog.BillingType,
		ServiceTier:                    cloneOptionalString(usageLog.ServiceTier),
		ReasoningEffort:                cloneOptionalString(usageLog.ReasoningEffort),
		User:                           input.User,
		APIKey:                         input.APIKey,
		Account:                        input.Account,
		Subscription:                   input.Subscription,
		APIKeyService:                  input.APIKeyService,
		Platform:                       input.QuotaPlatform,
		IsSubscriptionBill:             isSubscriptionBilling && !simpleModeKeyRateLimitOnly,
		SimpleModeKeyRateLimitOnly:     simpleModeKeyRateLimitOnly,
		AccountRateMultiplier:          accountRateMultiplier,
		RequestPayloadHash:             input.RequestPayloadHash,
		OriginalInputTokens:            usageLog.InputTokens,
		OriginalCacheCreationTokens:    usageLog.CacheCreationTokens,
		OriginalInputCost:              usageLog.InputCost,
		OriginalCacheCreationCost:      usageLog.CacheCreationCost,
		OriginalTotalCost:              usageLog.TotalCost,
		OriginalActualCost:             usageLog.ActualCost,
		OriginalAccountStatsCost:       cloneOptionalFloat64(usageLog.AccountStatsCost),
		DeltaInputCostPerToken:         probeCost.InputCost - cost.InputCost,
		DeltaCacheCreationCostPerToken: probeCost.CacheCreationCost - cost.CacheCreationCost,
		DeltaTotalCostPerToken:         probeCost.TotalCost - cost.TotalCost,
		DeltaActualCostPerToken:        probeCost.ActualCost - cost.ActualCost,
		DeltaAccountStatsCostPerToken:  deltaAccountStatsCost,
	}
}

func (s *OpenAIGatewayService) reconcileInferredCacheWrite(
	ctx context.Context,
	ready *openAICacheWriteReadyReconciliation,
) error {
	if s == nil || ready == nil {
		return nil
	}
	snapshot := ready.Snapshot
	inference := ready.Inference
	if inference.Epoch != snapshot.Epoch || !s.openAICacheWriteBillingEnabled(ctx, snapshot.Epoch) {
		// Disabled/old-epoch work is terminal, not eligible for a later re-enable.
		return nil
	}
	if snapshot.APIKey == nil || snapshot.User == nil || snapshot.Account == nil ||
		snapshot.RequestID == "" || snapshot.APIKey.ID <= 0 {
		return errors.New("cache write reconciliation snapshot is incomplete")
	}
	if inference.Tokens <= 0 || inference.Tokens > snapshot.OriginalInputTokens {
		return fmt.Errorf("cache write reconciliation tokens out of range: inferred=%d input=%d", inference.Tokens, snapshot.OriginalInputTokens)
	}

	writeTokens := inference.Tokens
	deltaTotal := snapshot.DeltaTotalCostPerToken * float64(writeTokens)
	deltaActual := snapshot.DeltaActualCostPerToken * float64(writeTokens)
	if deltaTotal < -openAICacheWriteBillingEpsilon || deltaActual < -openAICacheWriteBillingEpsilon {
		// Current OpenAI cache-write prices are >= ordinary input prices. A
		// negative correction means a custom/mutated price card would require a
		// refund path, which this conservative reconciler intentionally does not
		// perform.
		return fmt.Errorf("cache write reconciliation would require refund: delta_total=%g delta_actual=%g", deltaTotal, deltaActual)
	}
	if math.Abs(deltaTotal) < openAICacheWriteBillingEpsilon {
		deltaTotal = 0
	}
	if math.Abs(deltaActual) < openAICacheWriteBillingEpsilon {
		deltaActual = 0
	}

	correctedInputTokens := snapshot.OriginalInputTokens - writeTokens
	correctedCacheCreationTokens := snapshot.OriginalCacheCreationTokens + writeTokens
	correctedInputCost := snapshot.OriginalInputCost + snapshot.DeltaInputCostPerToken*float64(writeTokens)
	correctedCacheCreationCost := snapshot.OriginalCacheCreationCost + snapshot.DeltaCacheCreationCostPerToken*float64(writeTokens)
	correctedTotalCost := snapshot.OriginalTotalCost + deltaTotal
	// Monetary application is quantized to NUMERIC(20,8), matching the existing
	// billing command. usage_logs keeps the unquantized pricing calculation, just
	// like the original RecordUsage path, so its component math remains exact.
	quantizedDeltaActual := QuantizeUsageBillingAmount(deltaActual)
	correctedActualCost := snapshot.OriginalActualCost + deltaActual

	var correctedAccountStatsCost *float64
	if snapshot.OriginalAccountStatsCost != nil && snapshot.DeltaAccountStatsCostPerToken != nil {
		value := *snapshot.OriginalAccountStatsCost + *snapshot.DeltaAccountStatsCostPerToken*float64(writeTokens)
		correctedAccountStatsCost = &value
	}

	correction := &OpenAICacheWriteUsageCorrection{
		RequestID:                    snapshot.RequestID,
		APIKeyID:                     snapshot.APIKey.ID,
		OriginalInputTokens:          snapshot.OriginalInputTokens,
		OriginalCacheCreationTokens:  snapshot.OriginalCacheCreationTokens,
		CorrectedInputTokens:         correctedInputTokens,
		CorrectedCacheCreationTokens: correctedCacheCreationTokens,
		CorrectedInputCost:           correctedInputCost,
		CorrectedCacheCreationCost:   correctedCacheCreationCost,
		CorrectedTotalCost:           correctedTotalCost,
		CorrectedActualCost:          correctedActualCost,
		CorrectedAccountStatsCost:    correctedAccountStatsCost,
	}

	// The delta bill and the absolute usage-row rewrite are committed in the
	// same UsageBillingRepository transaction. A process crash can therefore
	// never leave money and usage buckets on opposite sides of the correction.
	if s.usageBillingRepo == nil {
		return errors.New("usage billing repository is required for inferred cache write billing")
	}
	adjustmentLog := &UsageLog{
		Model:               snapshot.Model,
		BillingType:         snapshot.BillingType,
		CacheCreationTokens: writeTokens,
		ServiceTier:         cloneOptionalString(snapshot.ServiceTier),
		ReasoningEffort:     cloneOptionalString(snapshot.ReasoningEffort),
	}
	if snapshot.Subscription != nil {
		adjustmentLog.SubscriptionID = &snapshot.Subscription.ID
	}
	deltaCost := &CostBreakdown{
		InputCost:         snapshot.DeltaInputCostPerToken * float64(writeTokens),
		CacheCreationCost: snapshot.DeltaCacheCreationCostPerToken * float64(writeTokens),
		TotalCost:         deltaTotal,
		ActualCost:        quantizedDeltaActual,
		BillingMode:       string(BillingModeToken),
	}
	// A retained snapshot may outlive a failed original log insert. Restore
	// the frozen row before the atomic bill/rewrite; Create is idempotent.
	if snapshot.OriginalUsageLog != nil {
		if _, err := s.createOpenAICacheWriteUsageLog(ctx, snapshot.OriginalUsageLog); err != nil {
			return fmt.Errorf("restore original cache write usage log: %w", err)
		}
	}
	if !s.openAICacheWriteBillingEnabled(ctx, snapshot.Epoch) {
		return nil
	}
	// The refreshing check above must finish before acquiring the read gate:
	// cache refresh needs its write side. This final pure-cache check and the
	// transaction are linearized against an acknowledged disable/epoch change.
	gatewayForwardingPublicationMu.RLock()
	defer gatewayForwardingPublicationMu.RUnlock()
	if !openAICacheWriteInferenceEnabledAtEpochLocked(snapshot.Epoch) {
		return nil
	}
	adjustmentID := openAICacheWriteAdjustmentRequestID(snapshot.RequestID, snapshot.APIKey.ID)
	applied, err := applyUsageBilling(ctx, adjustmentID, adjustmentLog, &postUsageBillingParams{
		Cost:                       deltaCost,
		CacheWriteCorrection:       correction,
		User:                       snapshot.User,
		APIKey:                     snapshot.APIKey,
		Account:                    snapshot.Account,
		Subscription:               snapshot.Subscription,
		RequestPayloadHash:         "cache-write-reconcile:" + strings.TrimSpace(snapshot.RequestPayloadHash),
		IsSubscriptionBill:         snapshot.IsSubscriptionBill,
		AccountRateMultiplier:      snapshot.AccountRateMultiplier,
		APIKeyService:              snapshot.APIKeyService,
		Platform:                   snapshot.Platform,
		SimpleModeKeyRateLimitOnly: snapshot.SimpleModeKeyRateLimitOnly,
	}, s.billingDeps(), s.usageBillingRepo)
	if err != nil {
		return err
	}
	logger.L().With(
		zap.String("component", "service.openai_gateway"),
		zap.String("adjustment_request_id", adjustmentID),
		zap.Bool("billing_applied", applied),
		zap.Int("inferred_cache_write_tokens", writeTokens),
		zap.Float64("delta_total_cost", deltaTotal),
		zap.Float64("delta_actual_cost", quantizedDeltaActual),
	).Debug("openai.cache_write_billing_adjusted")
	return nil
}

func (s *OpenAIGatewayService) processReadyOpenAICacheWriteReconciliations(ctx context.Context) {
	if s == nil {
		return
	}
	// Bound per-request reconciliation work. Additional ready items remain in
	// the tracker and are retried by later usage-record completions.
	for i := 0; i < 4; i++ {
		epoch := openAICacheWriteInferenceSettingEpoch.Load()
		if !s.openAICacheWriteBillingEnabled(ctx, epoch) {
			s.openaiCacheWriteInferenceTracker.clearAtSettingEpoch(openAICacheWriteInferenceSettingEpoch.Load())
			return
		}
		s.openaiCacheWriteInferenceTracker.resetForSettingEpoch(epoch)
		key, ready := s.openaiCacheWriteInferenceTracker.claimReadyReconciliation()
		if ready == nil {
			return
		}
		err := s.reconcileInferredCacheWrite(ctx, ready)
		s.openaiCacheWriteInferenceTracker.finishReadyReconciliation(key, err == nil)
		if err != nil {
			logger.L().With(
				zap.String("component", "service.openai_gateway"),
				zap.String("observation_id_sha256", hashSensitiveValueForLog(ready.ObservationID)),
				zap.Error(err),
			).Warn("openai.cache_write_billing_reconcile_failed")
			return
		}
	}
}
