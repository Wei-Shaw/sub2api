//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingRiskSettlementCacheStub struct {
	billingCacheWorkerStub
	deductCalls int
	amount      float64
}

func (s *billingRiskSettlementCacheStub) DeductUserBalance(_ context.Context, _ int64, amount float64) error {
	s.deductCalls++
	s.amount = amount
	return nil
}

func acquiredBillingRiskPermit(t *testing.T, store *billingRiskStoreStub) *BillingRiskPermit {
	t.Helper()
	guard := newBillingRiskGuardTest(t, true, store)
	permit, err := guard.Acquire(context.Background(), BillingRiskRequest{
		UserID:          41,
		Balance:         1,
		Kind:            BillingRiskRequestText,
		EstimatedCost:   0.1,
		EstimateCertain: true,
	})
	require.NoError(t, err)
	require.NotNil(t, permit)
	return permit
}

func billingRiskApplyParams(permit *BillingRiskPermit) (*postUsageBillingParams, *billingDeps, *UsageLog) {
	return &postUsageBillingParams{
			Cost:       &CostBreakdown{TotalCost: 0.1, ActualCost: 0.1},
			User:       &User{ID: 41, Balance: 1},
			APIKey:     &APIKey{ID: 42},
			Account:    &Account{ID: 43},
			RiskPermit: permit,
		}, &billingDeps{
			deferredService: NewDeferredService(nil, nil, time.Second),
		}, &UsageLog{UserID: 41, APIKeyID: 42, AccountID: 43, TotalCost: 0.1, ActualCost: 0.1}
}

func TestApplyUsageBillingCommitsRiskPermitWithTransactionalBalance(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, deps, usageLog := billingRiskApplyParams(permit)
	newBalance := 0.9
	repo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true, NewBalance: &newBalance}}

	applied, err := applyUsageBilling(context.Background(), "risk-success", usageLog, params, deps, repo)

	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, 1, store.commitCalls)
	require.Equal(t, int64(900_000), store.lastBalance)
	require.Zero(t, store.releaseCalls)
	require.Zero(t, store.uncertainCalls)
}

func TestApplyUsageBillingSynchronizesBalanceDeductionBeforeRiskPermitCommit(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, deps, usageLog := billingRiskApplyParams(permit)
	cache := &billingRiskSettlementCacheStub{}
	cacheService := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(cacheService.Stop)
	deps.billingCacheService = cacheService
	newBalance := 0.9
	repo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true, NewBalance: &newBalance}}
	commitSawFreshCache := false
	store.commitHook = func() {
		commitSawFreshCache = cache.deductCalls == 1 && cache.amount == params.Cost.ActualCost
	}

	applied, err := applyUsageBilling(context.Background(), "risk-cache-order", usageLog, params, deps, repo)

	require.NoError(t, err)
	require.True(t, applied)
	require.True(t, commitSawFreshCache, "开放风险预算前必须先同步扣减共享余额缓存")
	require.Equal(t, 1, cache.deductCalls)
	require.Equal(t, params.Cost.ActualCost, cache.amount)
}

func TestApplyUsageBillingMarksRiskPermitUncertainOnRepositoryError(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, deps, usageLog := billingRiskApplyParams(permit)
	repo := &openAIRecordUsageBillingRepoStub{err: errors.New("db unavailable")}

	applied, err := applyUsageBilling(context.Background(), "risk-error", usageLog, params, deps, repo)

	require.Error(t, err)
	require.False(t, applied)
	require.Equal(t, 1, store.uncertainCalls)
	require.Zero(t, store.commitCalls)
	require.Zero(t, store.releaseCalls)
}

func TestResolveBillingRiskPermitUsesLiveContextAfterBillingDeadline(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, _, _ := billingRiskApplyParams(permit)
	billingCtx, cancel := context.WithCancel(context.Background())
	cancel()

	resolveBillingRiskPermit(billingCtx, params, nil, false, errors.New("billing deadline exceeded"))

	require.Equal(t, 1, store.uncertainCalls)
	require.NoError(t, store.lastUncertainContextErr, "异常冷却必须使用独立的可用 context")
}

func TestOpenAIRecordUsageMarksHandedOffPermitUncertainOnEarlyError(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	svc := newOpenAIRecordUsageServiceForTest(
		&openAIRecordUsageLogRepoStub{},
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	svc.accountRepo = &openAIRecordUsageAccountRepoStub{err: errors.New("parent credential unavailable")}
	parentID := int64(99)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "risk-early-error",
			Model:     "gpt-5.1",
			Usage:     OpenAIUsage{InputTokens: 10, OutputTokens: 5},
		},
		APIKey:     &APIKey{ID: 42, UserID: 41, GroupID: i64p(1), Group: &Group{ID: 1, RateMultiplier: 1}},
		User:       &User{ID: 41, Balance: 1},
		Account:    &Account{ID: 43, ParentAccountID: &parentID},
		RiskPermit: permit,
	})

	require.Error(t, err)
	require.Equal(t, 1, store.uncertainCalls, "共享账务前早退必须终结已移交许可")
}

func TestRecordCyberPolicyUsageLogSettlesRiskPermit(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	newBalance := 0.9
	billingRepo := &openAIRecordUsageBillingRepoStub{
		result: &UsageBillingApplyResult{Applied: true, NewBalance: &newBalance},
	}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		&openAIRecordUsageLogRepoStub{inserted: true},
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)

	svc.RecordCyberPolicyUsageLog(context.Background(), CyberPolicyUsageInput{
		APIKey:       &APIKey{ID: 42, User: &User{ID: 41, Balance: 1}},
		Account:      &Account{ID: 43},
		RequestID:    "risk-cyber",
		Model:        "gpt-5.1",
		InputTokens:  10,
		OutputTokens: 5,
		RiskPermit:   permit,
	})

	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, store.commitCalls)
	require.Zero(t, store.releaseCalls)
}

func TestApplyUsageBillingReleasesRiskPermitWhenIdempotentApplyIsSkipped(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, deps, usageLog := billingRiskApplyParams(permit)
	repo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}

	applied, err := applyUsageBilling(context.Background(), "risk-duplicate", usageLog, params, deps, repo)

	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, 1, store.releaseCalls)
	require.Zero(t, store.commitCalls)
}

func TestApplyUsageBillingMarksRiskPermitUncertainWhenBalanceResultIsMissing(t *testing.T) {
	store := &billingRiskStoreStub{}
	permit := acquiredBillingRiskPermit(t, store)
	params, deps, usageLog := billingRiskApplyParams(permit)
	repo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}

	applied, err := applyUsageBilling(context.Background(), "risk-no-balance", usageLog, params, deps, repo)

	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, 1, store.uncertainCalls)
	require.Zero(t, store.commitCalls)
}

func TestUsageInputsCarryBillingRiskPermit(t *testing.T) {
	permit := &BillingRiskPermit{UserID: 41, LeaseID: "lease"}
	require.Same(t, permit, (&RecordUsageInput{RiskPermit: permit}).RiskPermit)
	require.Same(t, permit, (&RecordUsageLongContextInput{RiskPermit: permit}).RiskPermit)
	require.Same(t, permit, (&OpenAIRecordUsageInput{RiskPermit: permit}).RiskPermit)
}
