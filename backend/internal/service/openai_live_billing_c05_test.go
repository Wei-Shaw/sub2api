//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type liveBillingAPIKeySourceStub struct {
	mu     sync.Mutex
	apiKey *APIKey
	err    error
	gets   int
}

func (s *liveBillingAPIKeySourceStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	if s.err != nil {
		return nil, s.err
	}
	if s.apiKey == nil || s.apiKey.ID != id {
		return nil, ErrAPIKeyNotFound
	}
	copied := *s.apiKey
	return &copied, nil
}

func (s *liveBillingAPIKeySourceStub) UpdateQuotaUsed(context.Context, int64, float64) error {
	return nil
}

func (s *liveBillingAPIKeySourceStub) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

type liveBillingSubRepoStub struct {
	UserSubscriptionRepository
	sub *UserSubscription
}

func (s *liveBillingSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if s.sub == nil || s.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	copied := *s.sub
	return &copied, nil
}

type liveBillingUserGroupRateRepoStub struct {
	UserGroupRateRepository
	rates map[[2]int64]float64
}

func (s *liveBillingUserGroupRateRepoStub) GetByUserAndGroup(_ context.Context, userID, groupID int64) (*float64, error) {
	if rate, ok := s.rates[[2]int64{userID, groupID}]; ok {
		return &rate, nil
	}
	return nil, nil
}

type liveBillingC05Fixture struct {
	svc         *OpenAIGatewayService
	record      *LiveCallRecord
	usageRepo   *liveTestUsageRepo
	billingRepo *openAIRecordUsageBillingRepoStub
	keySource   *liveBillingAPIKeySourceStub
	releases    func() int
}

func newLiveBillingC05Fixture(
	t *testing.T,
	pricePerMinute float64,
	group *Group,
	subRepo UserSubscriptionRepository,
	rateRepo UserGroupRateRepository,
	subscriptionID int64,
) *liveBillingC05Fixture {
	t.Helper()
	const callID = "call_c05_billing"
	record := &LiveCallRecord{
		CallID:          callID,
		CallHash:        hashLiveCallID(callID),
		AccountID:       11,
		APIKeyID:        22,
		UserID:          33,
		GroupID:         group.ID,
		SubscriptionID:  subscriptionID,
		LeaseID:         "lease-c05",
		Model:           "gpt-live-test",
		CreatedAt:       time.Now().Add(-2 * time.Minute),
		ExpiresAt:       time.Now().Add(time.Hour),
		Controller:      LiveControllerPending,
		InboundEndpoint: "/v1/live",
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	billingRepo := &openAIRecordUsageBillingRepoStub{}

	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, nil, subRepo, rateRepo)
	svc.cache = store
	svc.concurrencyService = NewConcurrencyService(concurrencyCache)
	svc.cfg.Gateway.Live.PricePerMinuteUSD = pricePerMinute

	groupID := group.ID
	keySource := &liveBillingAPIKeySourceStub{apiKey: &APIKey{
		ID:          record.APIKeyID,
		Key:         "sk-live-c05",
		UserID:      record.UserID,
		GroupID:     &groupID,
		Group:       group,
		User:        &User{ID: record.UserID, Balance: 100},
		Quota:       100,
		RateLimit5h: 50,
		Status:      StatusActive,
	}}
	svc.SetLiveBillingAPIKeyService(keySource)

	return &liveBillingC05Fixture{
		svc:         svc,
		record:      record,
		usageRepo:   usageRepo,
		billingRepo: billingRepo,
		keySource:   keySource,
		releases: func() int {
			concurrencyCache.mu.Lock()
			defer concurrencyCache.mu.Unlock()
			return concurrencyCache.releases
		},
	}
}

func (f *liveBillingC05Fixture) singleLog(t *testing.T) *UsageLog {
	t.Helper()
	f.usageRepo.mu.Lock()
	defer f.usageRepo.mu.Unlock()
	require.Len(t, f.usageRepo.logs, 1)
	return f.usageRepo.logs[0]
}

// liveExpectedTotalCost 由用量行实际落库的 DurationMs 反推原始费用（按分钟小数计费，不取整）。
func liveExpectedTotalCost(t *testing.T, log *UsageLog, pricePerMinute float64) float64 {
	t.Helper()
	require.NotNil(t, log.DurationMs)
	return pricePerMinute * float64(*log.DurationMs) / float64(time.Minute/time.Millisecond)
}

func TestFinalizeLiveCallBillsPerMinuteInBalanceMode_C05(t *testing.T) {
	group := &Group{
		ID:               44,
		Platform:         PlatformOpenAI,
		RateMultiplier:   1.5,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	f := newLiveBillingC05Fixture(t, 0.5, group, nil, nil, 0)

	f.svc.finalizeLiveCall(f.record)
	f.svc.finalizeLiveCall(f.record)

	require.Equal(t, 1, f.releases())
	require.Equal(t, 1, f.billingRepo.calls, "Live 会话结束时必须且只能走一次计费管道")
	cmd := f.billingRepo.lastCmd
	require.NotNil(t, cmd)
	require.Equal(t, f.record.CallHash, cmd.RequestID)
	require.Equal(t, f.record.APIKeyID, cmd.APIKeyID)
	require.Equal(t, f.record.UserID, cmd.UserID)
	require.Equal(t, f.record.AccountID, cmd.AccountID)

	log := f.singleLog(t)
	totalCost := liveExpectedTotalCost(t, log, 0.5)
	// 2 分钟 × 0.5 USD/min ≈ 1.0 USD（加上测试本身的毫秒级耗时）。
	require.InDelta(t, 1.0, totalCost, 0.01)
	actualCost := totalCost * 1.5

	require.InDelta(t, actualCost, cmd.BalanceCost, 1e-7)
	require.Zero(t, cmd.SubscriptionCost)
	require.Nil(t, cmd.SubscriptionID)
	require.InDelta(t, actualCost, cmd.APIKeyQuotaCost, 1e-7)
	require.InDelta(t, actualCost, cmd.APIKeyRateLimitCost, 1e-7)

	require.Equal(t, RequestTypeLive, log.RequestType)
	require.Equal(t, f.record.CallHash, log.RequestID)
	require.Equal(t, BillingTypeBalance, log.BillingType)
	require.InDelta(t, totalCost, log.TotalCost, 1e-9)
	require.InDelta(t, actualCost, log.ActualCost, 1e-9)
	require.InDelta(t, 1.5, log.RateMultiplier, 1e-12)
	require.Zero(t, log.InputTokens)
	require.Zero(t, log.OutputTokens)
}

func TestFinalizeLiveCallBillsPerMinuteInSubscriptionModeWithUserRate_C05(t *testing.T) {
	group := &Group{
		ID:               45,
		Platform:         PlatformOpenAI,
		RateMultiplier:   1.5,
		SubscriptionType: SubscriptionTypeSubscription,
		Status:           StatusActive,
	}
	subRepo := &liveBillingSubRepoStub{sub: &UserSubscription{ID: 55, UserID: 33, GroupID: 45, Status: SubscriptionStatusActive}}
	// 用户专属倍率 2.0 覆盖分组默认 1.5，与普通请求的倍率解析一致。
	rateRepo := &liveBillingUserGroupRateRepoStub{rates: map[[2]int64]float64{{33, 45}: 2.0}}
	f := newLiveBillingC05Fixture(t, 0.5, group, subRepo, rateRepo, 55)

	f.svc.finalizeLiveCall(f.record)
	f.svc.finalizeLiveCall(f.record)

	require.Equal(t, 1, f.billingRepo.calls)
	cmd := f.billingRepo.lastCmd
	require.NotNil(t, cmd)
	require.Equal(t, f.record.CallHash, cmd.RequestID)

	log := f.singleLog(t)
	totalCost := liveExpectedTotalCost(t, log, 0.5)
	actualCost := totalCost * 2.0

	require.NotNil(t, cmd.SubscriptionID)
	require.Equal(t, int64(55), *cmd.SubscriptionID)
	require.InDelta(t, actualCost, cmd.SubscriptionCost, 1e-7)
	require.Zero(t, cmd.BalanceCost)

	require.Equal(t, BillingTypeSubscription, log.BillingType)
	require.NotNil(t, log.SubscriptionID)
	require.Equal(t, int64(55), *log.SubscriptionID)
	require.InDelta(t, totalCost, log.TotalCost, 1e-9)
	require.InDelta(t, actualCost, log.ActualCost, 1e-9)
	require.InDelta(t, 2.0, log.RateMultiplier, 1e-12)
}

func TestFinalizeLiveCallBillingFailureStillWritesUsageLog_C05(t *testing.T) {
	group := &Group{
		ID:               46,
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	f := newLiveBillingC05Fixture(t, 0.5, group, nil, nil, 0)
	f.billingRepo.err = errors.New("billing db down")

	f.svc.finalizeLiveCall(f.record)

	require.Equal(t, 1, f.billingRepo.calls)
	// 与普通请求 RecordUsage 一致：扣费失败仍落用量行（唯一落库机会），ActualCost 置 0 表示未扣。
	log := f.singleLog(t)
	require.InDelta(t, liveExpectedTotalCost(t, log, 0.5), log.TotalCost, 1e-9)
	require.Greater(t, log.TotalCost, 0.0)
	require.Zero(t, log.ActualCost)
}

func TestFinalizeLiveCallZeroPriceSkipsBilling_C05(t *testing.T) {
	group := &Group{
		ID:               47,
		Platform:         PlatformOpenAI,
		RateMultiplier:   1.5,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	f := newLiveBillingC05Fixture(t, 0, group, nil, nil, 0)

	f.svc.finalizeLiveCall(f.record)

	require.Zero(t, f.billingRepo.calls)
	require.Zero(t, f.keySource.gets, "价格为 0 时不应回查 API Key")
	log := f.singleLog(t)
	require.Zero(t, log.TotalCost)
	require.Zero(t, log.ActualCost)
	require.Equal(t, 1.0, log.RateMultiplier)
}
