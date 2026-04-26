//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func newGatewayRecordUsageServiceForTest(usageRepo UsageLogRepository, userRepo UserRepository, subRepo UserSubscriptionRepository) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1.1
	return NewGatewayService(
		nil,
		nil,
		usageRepo,
		nil,
		userRepo,
		subRepo,
		nil,
		nil,
		cfg,
		nil,
		nil,
		NewBillingService(cfg, nil),
		nil,
		&BillingCacheService{},
		nil,
		nil,
		&DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func newGatewayChannelResolverForTest(t *testing.T, pricing []ChannelModelPricing) *ModelPricingResolver {
	t.Helper()
	const groupID = 100
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{{
				ID:           1,
				Name:         "test-channel",
				Status:       StatusActive,
				GroupIDs:     []int64{groupID},
				ModelPricing: pricing,
			}}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{groupID: "anthropic"}, nil
		},
	}
	cs := NewChannelService(repo, nil, nil, nil)
	bs := newTestBillingServiceForResolver()
	return NewModelPricingResolver(cs, bs)
}

func newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo UsageLogRepository, billingRepo UsageBillingRepository, userRepo UserRepository, subRepo UserSubscriptionRepository) *GatewayService {
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)
	svc.usageBillingRepo = billingRepo
	return svc
}

type openAIRecordUsageBestEffortLogRepoStub struct {
	UsageLogRepository

	bestEffortErr   error
	createErr       error
	bestEffortCalls int
	createCalls     int
	lastLog         *UsageLog
	lastCtxErr      error
}

func (s *openAIRecordUsageBestEffortLogRepoStub) CreateBestEffort(ctx context.Context, log *UsageLog) error {
	s.bestEffortCalls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return s.bestEffortErr
}

func (s *openAIRecordUsageBestEffortLogRepoStub) Create(ctx context.Context, log *UsageLog) (bool, error) {
	s.createCalls++
	s.lastLog = log
	s.lastCtxErr = ctx.Err()
	return false, s.createErr
}

func TestGatewayServiceRecordUsage_BillingUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceeded}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsage(reqCtx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_detached_ctx",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    501,
			Quota: 100,
		},
		User:          &User{ID: 601},
		Account:       &Account{ID: 701},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.NoError(t, userRepo.lastCtxErr)
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.NoError(t, quotaSvc.lastQuotaCtxErr)
}

func TestGatewayServiceRecordUsage_BillingFingerprintIncludesRequestPayloadHash(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	payloadHash := HashUsageRequestPayload([]byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_payload_hash",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:             &APIKey{ID: 501, Quota: 100},
		User:               &User{ID: 601},
		Account:            &Account{ID: 701},
		RequestPayloadHash: payloadHash,
	})
	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, payloadHash, billingRepo.lastCmd.RequestPayloadHash)
}

func TestGatewayServiceRecordUsage_BillingFingerprintFallsBackToContextRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-local-123")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_payload_fallback",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})
	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "local:req-local-123", billingRepo.lastCmd.RequestPayloadHash)
}

func TestGatewayServiceRecordUsage_PreservesRequestedAndUpstreamModels(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	mappedModel := "claude-sonnet-4-20250514"

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:     "gateway_models_split",
			Usage:         ClaudeUsage{InputTokens: 10, OutputTokens: 6},
			Model:         "claude-sonnet-4",
			UpstreamModel: mappedModel,
			Duration:      time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "claude-sonnet-4", usageRepo.lastLog.Model)
	require.Equal(t, "claude-sonnet-4", usageRepo.lastLog.RequestedModel)
	require.NotNil(t, usageRepo.lastLog.UpstreamModel)
	require.Equal(t, mappedModel, *usageRepo.lastLog.UpstreamModel)
}

func TestGatewayServiceRecordUsage_PersistsChannelAndImageOutputFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	svc.resolver = newGatewayChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"},
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(10e-6),
		OutputPrice: testPtrFloat64(50e-6),
	}})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_channel_image_fields",
			Usage: ClaudeUsage{
				InputTokens:       10,
				OutputTokens:      6,
				ImageOutputTokens: 2,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 511, Quota: 100, GroupID: groupIDPtr(), Group: &Group{ID: 100}},
		User:    &User{ID: 611},
		Account: &Account{ID: 711},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      "alias-model",
			ChannelMappedModel: "claude-sonnet-4",
			BillingModelSource: BillingModelSourceChannelMapped,
			ModelMappingChain:  "alias-model→claude-sonnet-4",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 2, usageRepo.lastLog.ImageOutputTokens)
	require.NotZero(t, usageRepo.lastLog.ImageOutputCost)
	require.NotNil(t, usageRepo.lastLog.ChannelID)
	require.Equal(t, int64(9), *usageRepo.lastLog.ChannelID)
	require.NotNil(t, usageRepo.lastLog.ModelMappingChain)
	require.Equal(t, "alias-model→claude-sonnet-4", *usageRepo.lastLog.ModelMappingChain)
	require.NotNil(t, usageRepo.lastLog.BillingTier)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.BillingTier)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeToken), *usageRepo.lastLog.BillingMode)
}

func TestGatewayServiceRecordUsage_ChannelTokenPricingOverridesImageBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	svc.resolver = newGatewayChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"claude-sonnet-4"},
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(10e-6),
		OutputPrice: testPtrFloat64(50e-6),
	}})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:  "gateway_channel_token_pricing_overrides_image",
			Usage:      ClaudeUsage{InputTokens: 10, OutputTokens: 6, ImageOutputTokens: 2},
			Model:      "claude-sonnet-4",
			ImageCount: 1,
			ImageSize:  "2K",
			Duration:   time.Second,
		},
		APIKey:  &APIKey{ID: 511, Quota: 100, GroupID: groupIDPtr(), Group: &Group{ID: 100}},
		User:    &User{ID: 611},
		Account: &Account{ID: 711},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      "alias-model",
			ChannelMappedModel: "claude-sonnet-4",
			BillingModelSource: BillingModelSourceChannelMapped,
			ModelMappingChain:  "alias-model→claude-sonnet-4",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeToken), *usageRepo.lastLog.BillingMode)
	require.NotNil(t, usageRepo.lastLog.BillingTier)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.BillingTier)
	require.InDelta(t, 0.0001, usageRepo.lastLog.InputCost, 1e-12)
	require.InDelta(t, 0.0002, usageRepo.lastLog.OutputCost, 1e-12)
	require.InDelta(t, 0.0001, usageRepo.lastLog.ImageOutputCost, 1e-12)
	require.InDelta(t, 0.0004, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestGatewayServiceRecordUsage_ChannelImagePricingUsesRequestTier(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	svc.resolver = newGatewayChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:        "anthropic",
		Models:          []string{"claude-sonnet-4"},
		BillingMode:     BillingModeImage,
		PerRequestPrice: testPtrFloat64(0.08),
		Intervals: []PricingInterval{
			{TierLabel: "1K", PerRequestPrice: testPtrFloat64(0.04)},
			{TierLabel: "2K", PerRequestPrice: testPtrFloat64(0.08)},
			{TierLabel: "4K", PerRequestPrice: testPtrFloat64(0.16)},
		},
	}})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:  "gateway_channel_image_pricing_uses_request_tier",
			Usage:      ClaudeUsage{InputTokens: 10, OutputTokens: 6},
			Model:      "claude-sonnet-4",
			ImageCount: 2,
			ImageSize:  "2K",
			MediaType:  "image",
			Duration:   time.Second,
		},
		APIKey:  &APIKey{ID: 512, Quota: 100, GroupID: groupIDPtr(), Group: &Group{ID: 100}},
		User:    &User{ID: 612},
		Account: &Account{ID: 712},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          10,
			OriginalModel:      "claude-sonnet-4",
			ChannelMappedModel: "claude-sonnet-4",
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
	require.NotNil(t, usageRepo.lastLog.BillingTier)
	require.Equal(t, PricingSourceChannel, *usageRepo.lastLog.BillingTier)
	require.InDelta(t, 0.16, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.16, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestGatewayServiceRecordUsage_StoresEffectivePricingBreakdownFields(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	svc.resolver = newGatewayChannelResolverForTest(t, []ChannelModelPricing{{
		Platform:       "anthropic",
		Models:         []string{"claude-sonnet-4"},
		BillingMode:    BillingModeToken,
		InputPrice:     testPtrFloat64(10e-6),
		OutputPrice:    testPtrFloat64(50e-6),
		CacheReadPrice: testPtrFloat64(2e-6),
	}})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_effective_pricing_breakdown",
			Usage: ClaudeUsage{
				InputTokens:          10,
				OutputTokens:         6,
				CacheReadInputTokens: 2,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 513, Quota: 100, GroupID: groupIDPtr(), Group: &Group{ID: 100, RateMultiplier: 1.25}},
		User:    &User{ID: 613},
		Account: &Account{ID: 713, RateMultiplier: testPtrFloat64(1.6)},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          11,
			OriginalModel:      "alias-model",
			ChannelMappedModel: "claude-sonnet-4",
			BillingModelSource: BillingModelSourceChannelMapped,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.AccountRateMultiplier)
	require.InDelta(t, 1.6, *usageRepo.lastLog.AccountRateMultiplier, 1e-12)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, 1.25, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.NotNil(t, usageRepo.lastLog.EffectiveInputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.NotNil(t, usageRepo.lastLog.EffectiveCacheReadUnitPrice)
	require.InDelta(t, 10e-6, *usageRepo.lastLog.EffectiveInputUnitPrice, 1e-12)
	require.InDelta(t, 50e-6, *usageRepo.lastLog.EffectiveOutputUnitPrice, 1e-12)
	require.InDelta(t, 2e-6, *usageRepo.lastLog.EffectiveCacheReadUnitPrice, 1e-12)
	require.Nil(t, usageRepo.lastLog.PricingSource)
	require.InDelta(t, usageRepo.lastLog.TotalCost*1.25, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestGatewayServiceRecordUsageWithLongContext_PersistsMediaTypeAndPricingSource(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	err := svc.RecordUsageWithLongContext(context.Background(), &RecordUsageLongContextInput{
		Result: &ForwardResult{
			RequestID: "gateway_long_context_metadata",
			Usage: ClaudeUsage{
				InputTokens:  250000,
				OutputTokens: 16,
			},
			Model:     "claude-sonnet-4",
			Duration:  time.Second,
			MediaType: "video",
		},
		APIKey:                &APIKey{ID: 514, Quota: 100},
		User:                  &User{ID: 614},
		Account:               &Account{ID: 714},
		LongContextThreshold:  200000,
		LongContextMultiplier: 2,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.MediaType)
	require.Equal(t, "video", *usageRepo.lastLog.MediaType)
	require.NotNil(t, usageRepo.lastLog.PricingSource)
	require.Equal(t, "long_context_pricing", *usageRepo.lastLog.PricingSource)
	require.NotNil(t, usageRepo.lastLog.EffectiveMultiplier)
	require.InDelta(t, 1.1, *usageRepo.lastLog.EffectiveMultiplier, 1e-12)
	require.Nil(t, usageRepo.lastLog.EffectiveInputUnitPrice)
	require.Nil(t, usageRepo.lastLog.EffectiveOutputUnitPrice)
	require.Nil(t, usageRepo.lastLog.EffectiveCacheReadUnitPrice)
}

func TestGatewayServiceRecordUsage_UsageLogWriteErrorDoesNotSkipBilling(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: MarkUsageLogCreateNotPersisted(context.Canceled)}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_not_persisted",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    503,
			Quota: 100,
		},
		User:          &User{ID: 603},
		Account:       &Account{ID: 703},
		APIKeyService: quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.Equal(t, 1, quotaSvc.quotaCalls)
}

func TestGatewayServiceRecordUsageWithLongContext_BillingUsesDetachedContext(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: false, err: context.DeadlineExceeded}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.RecordUsageWithLongContext(reqCtx, &RecordUsageLongContextInput{
		Result: &ForwardResult{
			RequestID: "gateway_long_context_detached_ctx",
			Usage: ClaudeUsage{
				InputTokens:  12,
				OutputTokens: 8,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:    502,
			Quota: 100,
		},
		User:                  &User{ID: 602},
		Account:               &Account{ID: 702},
		LongContextThreshold:  200000,
		LongContextMultiplier: 2,
		APIKeyService:         quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, userRepo.deductCalls)
	require.NoError(t, userRepo.lastCtxErr)
	require.Equal(t, 1, quotaSvc.quotaCalls)
	require.NoError(t, quotaSvc.lastQuotaCtxErr)
}

func TestGatewayServiceRecordUsage_UsesFallbackRequestIDForUsageLog(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, subRepo)

	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "gateway-local-fallback")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 504},
		User:    &User{ID: 604},
		Account: &Account{ID: 704},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "local:gateway-local-fallback", usageRepo.lastLog.RequestID)
}

func TestGatewayServiceRecordUsage_PrefersClientRequestIDOverUpstreamRequestID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-stable-123")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "req-local-ignored")
	err := svc.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "upstream-volatile-456",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 506},
		User:    &User{ID: 606},
		Account: &Account{ID: 706},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "client:client-stable-123", billingRepo.lastCmd.RequestID)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "client:client-stable-123", usageRepo.lastLog.RequestID)
}

func TestGatewayServiceRecordUsage_GeneratesRequestIDWhenAllSourcesMissing(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 507},
		User:    &User{ID: 607},
		Account: &Account{ID: 707},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.True(t, strings.HasPrefix(billingRepo.lastCmd.RequestID, "generated:"))
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, billingRepo.lastCmd.RequestID, usageRepo.lastLog.RequestID)
}

func TestGatewayServiceRecordUsage_DroppedUsageLogDoesNotSyncFallback(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{
		bestEffortErr: MarkUsageLogCreateDropped(errors.New("usage log best-effort queue full")),
	}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_drop_usage_log",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 508},
		User:    &User{ID: 608},
		Account: &Account{ID: 708},
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.bestEffortCalls)
	require.Equal(t, 0, usageRepo.createCalls)
}

func TestGatewayServiceRecordUsage_BillingErrorSkipsUsageLogWrite(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{}
	billingRepo := &openAIRecordUsageBillingRepoStub{err: context.DeadlineExceeded}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_billing_fail",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 6,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 505},
		User:    &User{ID: 605},
		Account: &Account{ID: 705},
	})

	require.Error(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 0, usageRepo.calls)
}

func TestGatewayServiceRecordUsage_ReasoningEffortPersisted(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	effort := "max"
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "effort_test",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			Model:           "claude-opus-4-6",
			Duration:        time.Second,
			ReasoningEffort: &effort,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 1},
		Account: &Account{ID: 1},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.ReasoningEffort)
	require.Equal(t, "max", *usageRepo.lastLog.ReasoningEffort)
}

func TestGatewayServiceRecordUsage_ReasoningEffortNil(t *testing.T) {
	usageRepo := &openAIRecordUsageBestEffortLogRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "no_effort_test",
			Usage: ClaudeUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey:  &APIKey{ID: 1},
		User:    &User{ID: 1},
		Account: &Account{ID: 1},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Nil(t, usageRepo.lastLog.ReasoningEffort)
}
