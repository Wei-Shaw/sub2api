package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// openAICacheWriteUsageReconciler is optional so the broad UsageLogRepository
// interface and its many test doubles do not need a new method. Production's
// concrete usage-log repository implements it.
type openAICacheWriteUsageReconciler interface {
	ReconcileInferredCacheWrite(ctx context.Context, correction *OpenAICacheWriteUsageCorrection) (ready bool, err error)
}

type openAICacheWriteBillingSnapshot struct {
	ObservedAt    time.Time
	ObservationID string
	RequestID     string
	Model         string
	BillingType   int8
	ServiceTier   *string
	ReasoningEffort *string

	User         *User
	APIKey       *APIKey
	Account      *Account
	Subscription *UserSubscription
	APIKeyService APIKeyQuotaUpdater
	Platform      string

	IsSubscriptionBill          bool
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
	billingAccount *Account,
	usageLog *UsageLog,
	cost *CostBreakdown,
	tokens UsageTokens,
	billingModels []string,
	multiplier float64,
	imageMultiplier float64,
	videoMultiplier float64,
	baseMultiplier float64,
	serviceTier string,
	longContextBillingGate *bool,
	pricingAt time.Time,
	isSubscriptionBilling bool,
	accountRateMultiplier float64,
	simpleModeKeyRateLimitOnly bool,
) *openAICacheWriteBillingSnapshot {
	if s == nil || input == nil || input.Result == nil || usageLog == nil || cost == nil ||
		strings.TrimSpace(input.CacheWriteObservationID) == "" {
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

	probeCost, err := s.calculateOpenAIRecordUsageCost(
		ctx,
		result,
		input.APIKey,
		billingModels,
		multiplier,
		imageMultiplier,
		videoMultiplier,
		baseMultiplier,
		probeTokens,
		serviceTier,
		longContextBillingGate,
		pricingAt,
	)
	if err != nil || probeCost == nil {
		return nil
	}
	if groupBillsOpenAIFastAtStandard(input.APIKey, billingAccount, serviceTier) {
		standardProbeCost, standardErr := s.calculateOpenAIRecordUsageCost(
			ctx,
			result,
			input.APIKey,
			billingModels,
			multiplier,
			imageMultiplier,
			videoMultiplier,
			baseMultiplier,
			probeTokens,
			"",
			longContextBillingGate,
			pricingAt,
		)
		if standardErr != nil || standardProbeCost == nil {
			return nil
		}
		probeCost.ActualCost = standardProbeCost.ActualCost
	}

	var deltaAccountStatsCost *float64
	if usageLog.AccountStatsCost != nil && input.APIKey != nil && input.APIKey.GroupID != nil {
		probeLog := *usageLog
		probeLog.AccountStatsCost = nil
		applyAccountStatsCost(
			ctx,
			&probeLog,
			s.channelService,
			s.billingService,
			input.Account.ID,
			*input.APIKey.GroupID,
			result.UpstreamModel,
			result.Model,
			probeTokens,
			probeCost.TotalCost,
			pricingAt,
			accountStatsLongContextPricingEnabled(longContextBillingGate),
		)
		if probeLog.AccountStatsCost != nil {
			delta := *probeLog.AccountStatsCost - *usageLog.AccountStatsCost
			deltaAccountStatsCost = &delta
		}
	}

	return &openAICacheWriteBillingSnapshot{
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

	reconciler, ok := s.usageLogRepo.(openAICacheWriteUsageReconciler)
	if !ok || reconciler == nil {
		return errors.New("usage log repository does not support cache write reconciliation")
	}

	// Apply the money delta first using a separate deterministic request ID. It
	// is therefore idempotent and never conflicts with the original request's
	// billing fingerprint.
	if deltaTotal > 0 || quantizedDeltaActual > 0 {
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
		adjustmentID := openAICacheWriteAdjustmentRequestID(snapshot.RequestID, snapshot.APIKey.ID)
		applied, err := applyUsageBilling(ctx, adjustmentID, adjustmentLog, &postUsageBillingParams{
			Cost:                       deltaCost,
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
	}

	readyLog, err := reconciler.ReconcileInferredCacheWrite(ctx, &OpenAICacheWriteUsageCorrection{
		RequestID:                       snapshot.RequestID,
		APIKeyID:                        snapshot.APIKey.ID,
		OriginalInputTokens:             snapshot.OriginalInputTokens,
		OriginalCacheCreationTokens:     snapshot.OriginalCacheCreationTokens,
		CorrectedInputTokens:            correctedInputTokens,
		CorrectedCacheCreationTokens:    correctedCacheCreationTokens,
		CorrectedInputCost:              correctedInputCost,
		CorrectedCacheCreationCost:      correctedCacheCreationCost,
		CorrectedTotalCost:              correctedTotalCost,
		CorrectedActualCost:             correctedActualCost,
		CorrectedAccountStatsCost:       correctedAccountStatsCost,
	})
	if err != nil {
		return err
	}
	if !readyLog {
		return errors.New("original usage log is not available for cache write reconciliation yet")
	}
	return nil
}

func (s *OpenAIGatewayService) processReadyOpenAICacheWriteReconciliations(ctx context.Context) {
	if s == nil {
		return
	}
	// Bound per-request reconciliation work. Additional ready items remain in
	// the tracker and are retried by later usage-record completions.
	for i := 0; i < 4; i++ {
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
