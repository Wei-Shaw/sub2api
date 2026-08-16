//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type billingRiskAdmissionRateRepoStub struct {
	UserGroupRateRepository
	rate  *float64
	err   error
	calls int
}

func (s *billingRiskAdmissionRateRepoStub) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	s.calls++
	return s.rate, s.err
}

type billingRiskBudgetStoreStub struct {
	mu       sync.Mutex
	reserved int64
	calls    int
}

func (s *billingRiskBudgetStoreStub) Acquire(_ context.Context, request BillingRiskAcquireRequest) (*BillingRiskAcquireResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	budget := request.BalanceMicros - request.MinimumReserveMicros + request.OverdraftAllowanceMicros
	if s.reserved+request.RiskMicros > budget {
		return &BillingRiskAcquireResult{WouldReject: true}, nil
	}
	s.reserved += request.RiskMicros
	return &BillingRiskAcquireResult{Acquired: true, ReservedTotalMicros: s.reserved}, nil
}

func (s *billingRiskBudgetStoreStub) Refresh(context.Context, int64, string, time.Duration, time.Duration) (bool, error) {
	return true, nil
}

func (s *billingRiskBudgetStoreStub) Commit(context.Context, int64, string, int64, time.Duration) (bool, error) {
	return true, nil
}

func (s *billingRiskBudgetStoreStub) Release(context.Context, int64, string, time.Duration) (bool, error) {
	return true, nil
}

func (s *billingRiskBudgetStoreStub) MarkUncertain(context.Context, int64, string, int64, time.Duration, time.Duration) (bool, error) {
	return true, nil
}

func (s *billingRiskBudgetStoreStub) GetBalanceVersion(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *billingRiskBudgetStoreStub) ResetBalance(_ context.Context, _ int64, balance, _ int64, _ time.Duration) (*BillingRiskBalanceResetResult, error) {
	return &BillingRiskBalanceResetResult{Accepted: true, KnownBalanceMicros: balance}, nil
}

func newBillingRiskAdmissionTestService(t *testing.T, store BillingRiskStore, rateRepo UserGroupRateRepository) *BillingRiskAdmissionService {
	t.Helper()
	settings := NewSettingService(nil, &config.Config{})
	risk := DefaultBillingRiskSettings()
	risk.Enabled = true
	risk.SafetyFactor = 1
	risk.OverdraftAllowance = 0
	risk.MinimumRequestRisk = 0.000001
	settings.storeBillingRiskSettings(risk)
	guard := NewBillingRiskGuard(store, settings)
	return NewBillingRiskAdmissionService(guard, newBillingRiskEstimatorTest(), rateRepo, &config.Config{
		Default: config.DefaultConfig{RateMultiplier: 1},
		Billing: config.BillingConfig{MinimumBalanceReserve: 0},
	})
}

func billingRiskAdmissionAPIKey(balance float64, group *Group) *APIKey {
	groupID := group.ID
	return &APIKey{
		ID:      10,
		UserID:  20,
		GroupID: &groupID,
		Group:   group,
		User:    &User{ID: 20, Balance: balance},
	}
}

func TestBillingRiskAdmissionHighBalanceTextBypassesBeforeRateAndPricing(t *testing.T) {
	store := &billingRiskBudgetStoreStub{}
	rate := 9.0
	repo := &billingRiskAdmissionRateRepoStub{rate: &rate}
	admission := newBillingRiskAdmissionTestService(t, store, repo)

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          billingRiskAdmissionAPIKey(20, &Group{ID: 30, RateMultiplier: 2}),
		Kind:            BillingRiskRequestText,
		BillingModel:    "unknown-expensive-model",
		InputTokens:     1_000,
		MaxOutputTokens: 4_096,
	})

	require.NoError(t, err)
	require.Nil(t, permit)
	require.Zero(t, repo.calls, "高余额文本不应解析用户倍率")
	require.Zero(t, store.calls, "高余额文本不应访问 Redis")
}

func TestBillingRiskAdmissionHighBalanceAuthoritativeResetFailureStillBypasses(t *testing.T) {
	store := &billingRiskStoreStub{resetErr: errors.New("redis unavailable")}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	apiKey := billingRiskAdmissionAPIKey(20, &Group{ID: 30, RateMultiplier: 1})
	apiKey.User.BillingBalanceAuthoritative = true
	apiKey.User.BillingBalanceVersion = 3

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          apiKey,
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 4_096,
	})

	require.NoError(t, err)
	require.Nil(t, permit)
	require.Equal(t, 1, store.resetCalls)
	require.Equal(t, int64(3), store.lastResetVersion)
	require.Zero(t, store.acquireCalls)
}

func TestBillingRiskAdmissionSimpleModeBypassesBeforeRedis(t *testing.T) {
	store := &billingRiskBudgetStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	admission.cfg.RunMode = config.RunModeSimple

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          billingRiskAdmissionAPIKey(1, &Group{ID: 30, RateMultiplier: 1}),
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 4_096,
	})

	require.NoError(t, err)
	require.Nil(t, permit)
	require.Zero(t, store.calls, "simple 模式不应访问风险 Redis")
}

func TestBillingRiskAdmissionEnabledKeepsLocalErrors(t *testing.T) {
	admission := newBillingRiskAdmissionTestService(t, &billingRiskStoreStub{}, nil)

	_, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey: &APIKey{ID: 10, UserID: 20},
		Kind:   BillingRiskRequestSyncImage,
	})
	require.Equal(t, "INVALID_BILLING_RISK_REQUEST", infraerrors.Reason(err))

	_, err = admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey: billingRiskAdmissionAPIKey(1, &Group{ID: 30, RateMultiplier: 1}),
		Kind:   BillingRiskRequestText,
	})
	require.Equal(t, "INVALID_BILLING_RISK_ESTIMATE", infraerrors.Reason(err))
}

func TestBillingRiskAdmissionUsesCurrentBillingBalanceInsteadOfStaleAuthSnapshot(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	apiKey := billingRiskAdmissionAPIKey(20, &Group{ID: 30, RateMultiplier: 1})
	cache := &balanceEligibilityCacheStub{balance: 1}
	eligibility := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(eligibility.Stop)
	require.NoError(t, eligibility.CheckBillingEligibility(context.Background(), apiKey.User, apiKey, apiKey.Group, nil, ""))

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          apiKey,
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 500,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, 1, store.acquireCalls)
	require.Equal(t, int64(1_000_000), store.lastAcquire.BalanceMicros)
}

func TestBillingRiskAdmissionResetsKnownBalanceFromAuthoritativeDatabaseSnapshot(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	apiKey := billingRiskAdmissionAPIKey(0.75, &Group{ID: 30, RateMultiplier: 1})
	apiKey.User.BillingBalanceAuthoritative = true
	apiKey.User.BillingBalanceVersion = 5

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          apiKey,
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 500,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, 1, store.resetCalls)
	require.Equal(t, int64(750_000), store.lastResetBalance)
	require.Equal(t, int64(5), store.lastResetVersion)
	require.False(t, apiKey.User.BillingBalanceAuthoritative)
	require.Zero(t, apiKey.User.BillingBalanceVersion)
}

func TestBillingRiskAdmissionStaleDatabaseSnapshotCannotBypassWithHigherBalance(t *testing.T) {
	store := &billingRiskStoreStub{
		resetResult: &BillingRiskBalanceResetResult{Accepted: false, KnownBalanceMicros: 400_000},
	}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	apiKey := billingRiskAdmissionAPIKey(20, &Group{ID: 30, RateMultiplier: 1})
	apiKey.User.BillingBalanceAuthoritative = true
	apiKey.User.BillingBalanceVersion = 9

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          apiKey,
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 500,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, 1, store.resetCalls)
	require.Equal(t, 1, store.acquireCalls)
	require.Equal(t, int64(400_000), store.lastAcquire.BalanceMicros)
	require.Equal(t, 0.4, apiKey.User.Balance)
}

func TestBillingRiskAdmissionUserMultiplierIncreasesReservedRisk(t *testing.T) {
	baseStore := &billingRiskStoreStub{}
	base := newBillingRiskAdmissionTestService(t, baseStore, nil)
	group := &Group{ID: 31, RateMultiplier: 1}
	input := BillingRiskAdmissionInput{
		APIKey:          billingRiskAdmissionAPIKey(1, group),
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1_000,
		MaxOutputTokens: 500,
	}
	_, err := base.Acquire(context.Background(), input)
	require.NoError(t, err)

	rate := 4.0
	highStore := &billingRiskStoreStub{}
	high := newBillingRiskAdmissionTestService(t, highStore, &billingRiskAdmissionRateRepoStub{rate: &rate})
	_, err = high.Acquire(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, baseStore.lastAcquire.RiskMicros*4, highStore.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionMissingOutputLimitUsesConservative64KDefault(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)

	_, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(1, &Group{ID: 31, RateMultiplier: 1}),
		Kind:         BillingRiskRequestText,
		BillingModel: "claude-sonnet-4",
		InputTokens:  1_000,
	})

	require.NoError(t, err)
	const expectedRiskMicros = int64(1_000 + 64*1024*2)
	require.Equal(t, expectedRiskMicros, store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionRateLookupFailureUsesUnknownExclusiveRisk(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, &billingRiskAdmissionRateRepoStub{
		err: errors.New("rate repository unavailable"),
	})

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:          billingRiskAdmissionAPIKey(1, &Group{ID: 38, RateMultiplier: 0.1}),
		Kind:            BillingRiskRequestText,
		BillingModel:    "claude-sonnet-4",
		InputTokens:     1,
		MaxOutputTokens: 1,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(1_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionExpensiveMappedModelNeverAllowsMoreConcurrency(t *testing.T) {
	allowed := func(model string) int {
		store := &billingRiskBudgetStoreStub{}
		admission := newBillingRiskAdmissionTestService(t, store, nil)
		input := BillingRiskAdmissionInput{
			APIKey:          billingRiskAdmissionAPIKey(1, &Group{ID: 32, RateMultiplier: 1}),
			Kind:            BillingRiskRequestText,
			BillingModel:    model,
			InputTokens:     1_000,
			MaxOutputTokens: 500,
		}
		count := 0
		for {
			permit, err := admission.Acquire(context.Background(), input)
			if infraerrors.Reason(err) == "BILLING_RISK_BUDGET_EXCEEDED" {
				return count
			}
			require.NoError(t, err)
			require.NotNil(t, permit)
			count++
			require.Less(t, count, 10_000)
		}
	}

	cheapConcurrency := allowed("claude-sonnet-4")
	expensiveConcurrency := allowed("claude-opus-4.5")
	require.Positive(t, expensiveConcurrency)
	require.Less(t, expensiveConcurrency, cheapConcurrency)
}

func TestBillingRiskAdmissionSupportsNoOutputAndConservativeUnknown(t *testing.T) {
	group := &Group{ID: 33, RateMultiplier: 1}
	apiKey := billingRiskAdmissionAPIKey(1, group)

	inputOnlyStore := &billingRiskStoreStub{}
	inputOnly := newBillingRiskAdmissionTestService(t, inputOnlyStore, nil)
	_, err := inputOnly.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       apiKey,
		Kind:         BillingRiskRequestText,
		BillingModel: "claude-sonnet-4",
		InputTokens:  1_000,
		NoOutput:     true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1_000), inputOnlyStore.lastAcquire.RiskMicros)

	unknownStore := &billingRiskStoreStub{}
	unknown := newBillingRiskAdmissionTestService(t, unknownStore, nil)
	_, err = unknown.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:              apiKey,
		Kind:                BillingRiskRequestSyncImage,
		BillingModel:        "claude-sonnet-4",
		InputTokens:         1_000,
		ConservativeUnknown: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1_000_000), unknownStore.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionConservativeUnknownDoesNotUseHighBalanceTextBypass(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:              billingRiskAdmissionAPIKey(20, &Group{ID: 45, RateMultiplier: 1}),
		Kind:                BillingRiskRequestText,
		BillingModel:        "unknown-at-admission",
		ConservativeUnknown: true,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(20_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskPermitSnapshotRestoresLifecycleAfterGatewayRestart(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:              billingRiskAdmissionAPIKey(5, &Group{ID: 34, RateMultiplier: 1}),
		Kind:                BillingRiskRequestVideo,
		BillingModel:        "unknown-video-model",
		ConservativeUnknown: true,
		ForceProtect:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, permit)

	snapshot := permit.Snapshot()
	require.Equal(t, permit.LeaseID, snapshot.LeaseID)
	require.Equal(t, int64(20), snapshot.UserID)
	restored := admission.RestorePermit(snapshot)
	require.NotNil(t, restored)
	require.NoError(t, restored.Release(context.Background()))
	require.Equal(t, 1, store.releaseCalls)
}

func TestBillingRiskAdmissionSearchPriceControlsHighBalanceProtection(t *testing.T) {
	lowStore := &billingRiskStoreStub{}
	low := newBillingRiskAdmissionTestService(t, lowStore, nil)
	lowGroup := &Group{ID: 35, RateMultiplier: 1}
	permit, err := low.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, lowGroup),
		Kind:         BillingRiskRequestSearch,
		BillingModel: "grok-web-search",
		SearchCalls:  1,
	})
	require.NoError(t, err)
	require.Nil(t, permit)
	require.Zero(t, lowStore.acquireCalls)

	highPrice := 2_000.0
	highStore := &billingRiskStoreStub{}
	high := newBillingRiskAdmissionTestService(t, highStore, nil)
	highGroup := &Group{ID: 36, RateMultiplier: 1, SearchPricePer1k: &highPrice}
	permit, err = high.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, highGroup),
		Kind:         BillingRiskRequestSearch,
		BillingModel: "grok-web-search",
		SearchCalls:  1,
	})
	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(2_000_000), highStore.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionAudioPriceControlsHighBalanceProtection(t *testing.T) {
	price := 1_000.0
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{ID: 37, RateMultiplier: 1, AudioTTSPricePerMillionChars: &price}

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, group),
		Kind:         BillingRiskRequestAudio,
		BillingModel: "grok-voice-latest",
		AudioMode:    "tts",
		UsageUnits:   0.002,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(2_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionAudioUsesModelPerRequestPriceBeforeFlatPrice(t *testing.T) {
	perRequestPrice := 5.0
	flatPrice := 1.0
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{
		ID:                           43,
		RateMultiplier:               1,
		AudioTTSPricePerMillionChars: &flatPrice,
		ModelPricing: []ChannelModelPricing{{
			Models:          []string{"grok-voice-latest"},
			BillingMode:     BillingModePerRequest,
			PerRequestPrice: &perRequestPrice,
		}},
	}

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, group),
		Kind:         BillingRiskRequestAudio,
		BillingModel: "grok-voice-latest",
		AudioMode:    "tts",
		UsageUnits:   1,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(5_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionAppliesFixedLongContextPricing(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{ID: 44, RateMultiplier: 1, LongContextPricingEnabled: true}
	tokens := UsageTokens{InputTokens: 210_000, OutputTokens: 64 * 1024}
	expected, err := admission.estimator.billing.CalculateCostWithLongContext(
		"claude-sonnet-4",
		tokens,
		1,
		200_000,
		2,
	)
	require.NoError(t, err)

	_, err = admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:                billingRiskAdmissionAPIKey(1, group),
		Kind:                  BillingRiskRequestText,
		BillingModel:          "claude-sonnet-4",
		InputTokens:           tokens.InputTokens,
		LongContextThreshold:  200_000,
		LongContextMultiplier: 2,
	})

	require.NoError(t, err)
	expectedRisk, err := billingRiskMicrosCeil(expected.ActualCost)
	require.NoError(t, err)
	require.Equal(t, expectedRisk, store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionUsesGroupFlatImagePriceBeforeTokenFallback(t *testing.T) {
	price := 5.0
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{ID: 39, RateMultiplier: 1, ImagePrice2K: &price}

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, group),
		Kind:         BillingRiskRequestSyncImage,
		BillingModel: "claude-sonnet-4",
		RequestCount: 2,
		SizeTier:     "2K",
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(10_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionUsesGroupFlatVideoPriceBeforeTokenFallback(t *testing.T) {
	pricePerSecond := 0.5
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{ID: 40, RateMultiplier: 1, VideoPrice720P: &pricePerSecond}

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, group),
		Kind:         BillingRiskRequestVideo,
		BillingModel: "claude-sonnet-4",
		RequestCount: 1,
		UsageUnits:   10,
		SizeTier:     "720p",
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(5_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionUsesDefaultMediaPriceInsteadOfZeroTokenEstimate(t *testing.T) {
	billing := newBillingRiskEstimatorTest().billing
	group := &Group{ID: 41, RateMultiplier: 1}
	apiKey := billingRiskAdmissionAPIKey(1, group)

	imageStore := &billingRiskStoreStub{}
	imageAdmission := newBillingRiskAdmissionTestService(t, imageStore, nil)
	_, err := imageAdmission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       apiKey,
		Kind:         BillingRiskRequestSyncImage,
		BillingModel: "claude-sonnet-4",
		RequestCount: 1,
		SizeTier:     "2K",
	})
	require.NoError(t, err)
	imageCost := billing.CalculateImageCost("claude-sonnet-4", "2K", 1, nil, 1).ActualCost
	imageRisk, err := billingRiskMicrosCeil(imageCost)
	require.NoError(t, err)
	require.Equal(t, imageRisk, imageStore.lastAcquire.RiskMicros)

	videoStore := &billingRiskStoreStub{}
	videoAdmission := newBillingRiskAdmissionTestService(t, videoStore, nil)
	_, err = videoAdmission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       apiKey,
		Kind:         BillingRiskRequestVideo,
		BillingModel: "claude-sonnet-4",
		RequestCount: 1,
		UsageUnits:   10,
		SizeTier:     "720p",
	})
	require.NoError(t, err)
	videoCost := billing.CalculateVideoCost("claude-sonnet-4", "720p", 1, 10, nil, 1).ActualCost
	videoRisk, err := billingRiskMicrosCeil(videoCost)
	require.NoError(t, err)
	require.Equal(t, videoRisk, videoStore.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionTokenBilledMediaUsesUnknownExclusiveRisk(t *testing.T) {
	imageOutputPrice := 25.0
	for _, tc := range []struct {
		name       string
		kind       BillingRiskRequestKind
		usageUnits float64
		sizeTier   string
	}{
		{name: "image", kind: BillingRiskRequestSyncImage, sizeTier: "2K"},
		{name: "video", kind: BillingRiskRequestVideo, usageUnits: 10, sizeTier: "720p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &billingRiskStoreStub{}
			admission := newBillingRiskAdmissionTestService(t, store, nil)
			group := &Group{
				ID:             42,
				RateMultiplier: 1,
				ModelPricing: []ChannelModelPricing{{
					Models:           []string{"claude-sonnet-4"},
					BillingMode:      BillingModeToken,
					ImageOutputPrice: &imageOutputPrice,
				}},
			}

			permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
				APIKey:       billingRiskAdmissionAPIKey(20, group),
				Kind:         tc.kind,
				BillingModel: "claude-sonnet-4",
				InputTokens:  1,
				RequestCount: 1,
				UsageUnits:   tc.usageUnits,
				SizeTier:     tc.sizeTier,
			})

			require.NoError(t, err)
			require.NotNil(t, permit)
			require.Equal(t, 1, store.acquireCalls)
			require.Equal(t, int64(20_000_000), store.lastAcquire.RiskMicros)
		})
	}
}

func TestBillingRiskAdmissionChannelImageModeVideoUsesRequestCountInsteadOfDuration(t *testing.T) {
	store := &billingRiskStoreStub{}
	admission := newBillingRiskAdmissionTestService(t, store, nil)
	group := &Group{ID: 46, RateMultiplier: 1}
	admission.estimator.resolver = newOpenAIImageChannelPricingResolverForTest(t, group.ID, "grok-imagine-video", 1)

	permit, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
		APIKey:       billingRiskAdmissionAPIKey(20, group),
		Kind:         BillingRiskRequestVideo,
		BillingModel: "grok-imagine-video",
		RequestCount: 1,
		UsageUnits:   8,
		SizeTier:     "720p",
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(1_000_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskAdmissionUsesFinalBillingMultiplierPolicy(t *testing.T) {
	pricingAt := time.Date(2026, 8, 16, 12, 0, 0, 0, time.Local)
	peakGroup := func(id int64) *Group {
		return &Group{
			ID:                 id,
			RateMultiplier:     1,
			SubscriptionType:   SubscriptionTypeSubscription,
			PeakRateEnabled:    true,
			PeakStart:          "00:00",
			PeakEnd:            "23:59",
			PeakRateMultiplier: 3,
		}
	}

	t.Run("Grok audio uses base multiplier", func(t *testing.T) {
		price := 1_000.0
		group := peakGroup(47)
		group.AudioTTSPricePerMillionChars = &price
		store := &billingRiskStoreStub{}
		admission := newBillingRiskAdmissionTestService(t, store, nil)

		_, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
			APIKey:       billingRiskAdmissionAPIKey(20, group),
			Kind:         BillingRiskRequestAudio,
			BillingModel: "grok-voice-latest",
			AudioMode:    "tts",
			UsageUnits:   0.001,
			PricingAt:    pricingAt,
		})

		require.NoError(t, err)
		require.Equal(t, int64(1_000_000), store.lastAcquire.RiskMicros)
	})

	t.Run("Alpha Search uses base multiplier", func(t *testing.T) {
		price := 1.0
		group := peakGroup(48)
		group.WebSearchPricePerCall = &price
		store := &billingRiskStoreStub{}
		admission := newBillingRiskAdmissionTestService(t, store, nil)

		_, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
			APIKey:         billingRiskAdmissionAPIKey(20, group),
			Kind:           BillingRiskRequestSearch,
			BillingModel:   "gpt-5.1",
			WebSearchCalls: 1,
			PricingAt:      pricingAt,
		})

		require.NoError(t, err)
		require.Equal(t, int64(1_000_000), store.lastAcquire.RiskMicros)
	})

	t.Run("Gateway Search keeps peak text multiplier", func(t *testing.T) {
		price := 1_000.0
		group := peakGroup(49)
		group.SearchPricePer1k = &price
		store := &billingRiskStoreStub{}
		admission := newBillingRiskAdmissionTestService(t, store, nil)

		_, err := admission.Acquire(context.Background(), BillingRiskAdmissionInput{
			APIKey:       billingRiskAdmissionAPIKey(20, group),
			Kind:         BillingRiskRequestSearch,
			BillingModel: "grok-web-search",
			SearchCalls:  1,
			PricingAt:    pricingAt,
		})

		require.NoError(t, err)
		require.Equal(t, int64(3_000_000), store.lastAcquire.RiskMicros)
	})
}
