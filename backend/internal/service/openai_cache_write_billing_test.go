package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type cacheWriteBillingUsageRepoStub struct {
	UsageLogRepository
	logs        map[string]*UsageLog
	corrections []*OpenAICacheWriteUsageCorrection
}

func cacheWriteBillingUsageKey(requestID string, apiKeyID int64) string {
	return requestID + "|" + strconv.FormatInt(apiKeyID, 10)
}

func (s *cacheWriteBillingUsageRepoStub) Create(_ context.Context, log *UsageLog) (bool, error) {
	if s.logs == nil {
		s.logs = make(map[string]*UsageLog)
	}
	cloned := *log
	s.logs[cacheWriteBillingUsageKey(log.RequestID, log.APIKeyID)] = &cloned
	return true, nil
}

func (s *cacheWriteBillingUsageRepoStub) applyCorrection(correction *OpenAICacheWriteUsageCorrection) bool {
	if s == nil || correction == nil || s.logs == nil {
		return false
	}
	key := cacheWriteBillingUsageKey(correction.RequestID, correction.APIKeyID)
	log := s.logs[key]
	if log == nil {
		return false
	}
	cloned := *correction
	s.corrections = append(s.corrections, &cloned)
	log.InputTokens = correction.CorrectedInputTokens
	log.CacheCreationTokens = correction.CorrectedCacheCreationTokens
	log.InputCost = correction.CorrectedInputCost
	log.CacheCreationCost = correction.CorrectedCacheCreationCost
	log.TotalCost = correction.CorrectedTotalCost
	log.ActualCost = correction.CorrectedActualCost
	if correction.CorrectedAccountStatsCost != nil {
		value := *correction.CorrectedAccountStatsCost
		log.AccountStatsCost = &value
	}
	return true
}

type cacheWriteBillingRepoStub struct {
	UsageBillingRepository
	cmds      []*UsageBillingCommand
	seen      map[string]struct{}
	usageRepo *cacheWriteBillingUsageRepoStub
}

func (s *cacheWriteBillingRepoStub) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	cloned := *cmd
	s.cmds = append(s.cmds, &cloned)
	key := cmd.RequestID + "|" + strconv.FormatInt(cmd.APIKeyID, 10)
	if _, ok := s.seen[key]; ok {
		return &UsageBillingApplyResult{Applied: false}, nil
	}
	if cmd.CacheWriteCorrection != nil {
		if s.usageRepo == nil || !s.usageRepo.applyCorrection(cmd.CacheWriteCorrection) {
			return nil, errors.New("cache write correction target missing")
		}
	}
	s.seen[key] = struct{}{}
	return &UsageBillingApplyResult{Applied: true}, nil
}

func newCacheWriteBillingServiceForTest(
	usageRepo UsageLogRepository,
	billingRepo UsageBillingRepository,
) *OpenAIGatewayService {
	if billingStub, ok := billingRepo.(*cacheWriteBillingRepoStub); ok {
		if usageStub, usageOK := usageRepo.(*cacheWriteBillingUsageRepoStub); usageOK {
			billingStub.usageRepo = usageStub
		}
	}
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	svc := NewOpenAIGatewayService(
		nil,
		usageRepo,
		billingRepo,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	return svc
}

func TestOpenAICacheWriteBilling_ReconcilesPreviousTurnDelta(t *testing.T) {
	usageRepo := &cacheWriteBillingUsageRepoStub{}
	billingRepo := &cacheWriteBillingRepoStub{}
	svc := newCacheWriteBillingServiceForTest(usageRepo, billingRepo)

	groupID := int64(91)
	group := &Group{ID: groupID, RateMultiplier: 0.2}
	apiKey := &APIKey{ID: 501, GroupID: &groupID, Group: group}
	user := &User{ID: 601}
	account := &Account{ID: 701, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	result := &OpenAIForwardResult{
		RequestID: "req-cache-write-original",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:          154668,
			CacheReadInputTokens: 152400,
			// Subscription-backed Codex response omitted cache_write_tokens.
			CacheCreationInputTokensPresent: false,
		},
		Duration: time.Second,
	}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:                  result,
		APIKey:                  apiKey,
		User:                    user,
		Account:                 account,
		CacheWriteObservationID: "obs-original",
		RequestPayloadHash:      "payload-original",
	})
	require.NoError(t, err)
	require.Len(t, billingRepo.cmds, 1, "original request should be billed once before inference")

	originalLog := usageRepo.logs[cacheWriteBillingUsageKey(result.RequestID, apiKey.ID)]
	require.NotNil(t, originalLog)
	require.Equal(t, 2268, originalLog.InputTokens)
	require.Zero(t, originalLog.CacheCreationTokens)

	// The next rolling cache hit proves that 2,200 of the previous ordinary
	// input tokens were actually cache creation, leaving the observed 68-token
	// ordinary residual.
	svc.openaiCacheWriteInferenceTracker.registerInference("obs-original", openAICacheWriteInference{
		Tokens:              2200,
		ResidualInputTokens: 68,
		PreviousCacheRead:   152400,
		NextCacheRead:       154600,
		Source:              "inferred_next_hit",
	}, time.Now())
	svc.processReadyOpenAICacheWriteReconciliations(context.Background())

	require.Len(t, billingRepo.cmds, 2, "reconciliation should create exactly one delta bill")
	adjustment := billingRepo.cmds[1]
	require.NotEqual(t, result.RequestID, adjustment.RequestID, "delta must use its own dedup key")
	require.Contains(t, adjustment.RequestID, "cwr:")

	// Astra standard rates: ordinary input $10/MTok, cache write $12.5/MTok.
	// Moving 2200 tokens between those buckets adds $0.0055 upstream cost.
	// Group rate 0.2 means customer delta is $0.0011.
	require.InDelta(t, 0.0011, adjustment.BalanceCost, 1e-12)
	require.Zero(t, adjustment.SubscriptionCost)
	require.Zero(t, adjustment.APIKeyQuotaCost)
	require.Zero(t, adjustment.APIKeyRateLimitCost)
	require.Zero(t, adjustment.AccountQuotaCost)

	require.Len(t, usageRepo.corrections, 1)
	correction := usageRepo.corrections[0]
	require.Equal(t, 2268, correction.OriginalInputTokens)
	require.Zero(t, correction.OriginalCacheCreationTokens)
	require.Equal(t, 68, correction.CorrectedInputTokens)
	require.Equal(t, 2200, correction.CorrectedCacheCreationTokens)

	correctedLog := usageRepo.logs[cacheWriteBillingUsageKey(result.RequestID, apiKey.ID)]
	require.Equal(t, 68, correctedLog.InputTokens)
	require.Equal(t, 2200, correctedLog.CacheCreationTokens)
	require.InDelta(t, 68*10e-6, correctedLog.InputCost, 1e-12)
	require.InDelta(t, 2200*12.5e-6, correctedLog.CacheCreationCost, 1e-12)

	// Ready reconciliation is removed after success; retries cannot double-charge.
	svc.processReadyOpenAICacheWriteReconciliations(context.Background())
	require.Len(t, billingRepo.cmds, 2)
	require.Len(t, usageRepo.corrections, 1)
}

func TestOpenAICacheWriteBilling_AuthoritativeWriteNeverCreatesInferenceSnapshot(t *testing.T) {
	usageRepo := &cacheWriteBillingUsageRepoStub{}
	billingRepo := &cacheWriteBillingRepoStub{}
	svc := newCacheWriteBillingServiceForTest(usageRepo, billingRepo)

	apiKey := &APIKey{ID: 502, Group: &Group{RateMultiplier: 1}}
	result := &OpenAIForwardResult{
		RequestID: "req-authoritative-write",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:                     3200,
			CacheReadInputTokens:            1000,
			CacheCreationInputTokens:        2000,
			CacheCreationInputTokensPresent: true,
		},
	}

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:                  result,
		APIKey:                  apiKey,
		User:                    &User{ID: 602},
		Account:                 &Account{ID: 702, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		CacheWriteObservationID: "obs-authoritative",
	})
	require.NoError(t, err)
	require.Empty(t, svc.openaiCacheWriteInferenceTracker.billingSnapshots,
		"upstream-positive cache_write_tokens must remain authoritative")
}

func TestOpenAICacheWriteTracker_PairsInferenceAndBillingSnapshotInEitherOrder(t *testing.T) {
	t.Parallel()

	inference := openAICacheWriteInference{Tokens: 100, Source: "inferred_next_hit"}
	snapshot := openAICacheWriteBillingSnapshot{
		ObservedAt:    time.Now(),
		ObservationID: "obs",
		RequestID:     "req",
		OriginalInputTokens: 1000,
	}

	t.Run("snapshot then inference", func(t *testing.T) {
		tracker := &openAICacheWriteInferenceTracker{}
		tracker.attachBillingSnapshot("obs", snapshot)
		tracker.registerInference("obs", inference, time.Now())
		_, ready := tracker.claimReadyReconciliation()
		require.NotNil(t, ready)
		require.Equal(t, 100, ready.Inference.Tokens)
	})

	t.Run("inference then snapshot", func(t *testing.T) {
		tracker := &openAICacheWriteInferenceTracker{}
		tracker.registerInference("obs", inference, time.Now())
		tracker.attachBillingSnapshot("obs", snapshot)
		_, ready := tracker.claimReadyReconciliation()
		require.NotNil(t, ready)
		require.Equal(t, 100, ready.Inference.Tokens)
	})
}


func TestOpenAICacheWriteBilling_TwoTurnEndToEnd(t *testing.T) {
	resetOpenAICacheWriteInferenceSwitchForTest(t)

	usageRepo := &cacheWriteBillingUsageRepoStub{}
	billingRepo := &cacheWriteBillingRepoStub{}
	svc := newCacheWriteBillingServiceForTest(usageRepo, billingRepo)
	settingService := &SettingService{}
	svc.settingService = settingService
	settingService.refreshCachedSettings(&SystemSettings{OpenAICacheWriteInferenceEnabled: true})

	groupID := int64(92)
	group := &Group{ID: groupID, RateMultiplier: 0.2}
	apiKey := &APIKey{ID: 503, GroupID: &groupID, Group: group}
	user := &User{ID: 603}
	account := &Account{ID: 703, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	cacheIdentity := "session-two-turn"

	first := &OpenAIForwardResult{
		RequestID: "req-turn-1",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:          154668,
			CacheReadInputTokens: 152400,
		},
		Duration: time.Second,
	}
	firstObservationID := svc.ObserveOpenAICacheWriteTelemetry(
		context.Background(), account, first.Model, cacheIdentity, first,
	)
	require.Equal(t, first.RequestID, firstObservationID)
	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:                  first,
		APIKey:                  apiKey,
		User:                    user,
		Account:                 account,
		CacheWriteObservationID: firstObservationID,
		RequestPayloadHash:      "payload-turn-1",
	}))
	require.Len(t, billingRepo.cmds, 1)

	second := &OpenAIForwardResult{
		RequestID: "req-turn-2",
		Model:     "gpt-6-astra",
		Usage: OpenAIUsage{
			InputTokens:          156900,
			CacheReadInputTokens: 154600,
		},
		Duration: time.Second,
	}
	secondObservationID := svc.ObserveOpenAICacheWriteTelemetry(
		context.Background(), account, second.Model, cacheIdentity, second,
	)
	require.Equal(t, second.RequestID, secondObservationID)

	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:                  second,
		APIKey:                  apiKey,
		User:                    user,
		Account:                 account,
		CacheWriteObservationID: secondObservationID,
		RequestPayloadHash:      "payload-turn-2",
	}))

	require.Len(t, billingRepo.cmds, 3,
		"turn 1 + turn 2 normal bills plus one post-hoc cache-write delta")
	require.Equal(t, first.RequestID, billingRepo.cmds[0].RequestID)
	require.Equal(t, second.RequestID, billingRepo.cmds[1].RequestID)
	require.Contains(t, billingRepo.cmds[2].RequestID, "cwr:")
	require.InDelta(t, 0.0011, billingRepo.cmds[2].BalanceCost, 1e-12)

	firstLog := usageRepo.logs[cacheWriteBillingUsageKey(first.RequestID, apiKey.ID)]
	require.NotNil(t, firstLog)
	require.Equal(t, 68, firstLog.InputTokens)
	require.Equal(t, 2200, firstLog.CacheCreationTokens)

	secondLog := usageRepo.logs[cacheWriteBillingUsageKey(second.RequestID, apiKey.ID)]
	require.NotNil(t, secondLog)
	require.Equal(t, 2300, secondLog.InputTokens)
	require.Zero(t, secondLog.CacheCreationTokens)
}
