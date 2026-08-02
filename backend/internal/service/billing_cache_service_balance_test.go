//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type balanceEligibilityCacheStub struct {
	billingCacheWorkerStub

	balance                  float64
	cacheMissAfterInvalidate bool
	invalidated              atomic.Bool
	deductCalls              atomic.Int64
	invalidateCalls          atomic.Int64
	lastUserID               atomic.Int64
}

func (s *balanceEligibilityCacheStub) GetUserBalance(_ context.Context, userID int64) (float64, error) {
	s.lastUserID.Store(userID)
	if s.cacheMissAfterInvalidate && s.invalidated.Load() {
		return 0, errors.New("cache miss")
	}
	return s.balance, nil
}

type billingEligibilityResolverStub struct {
	result                   *BillingContext
	err                      error
	organizationBalance      float64
	organizationBalanceErr   error
	spendLimitErr            error
	calls                    atomic.Int64
	organizationBalanceCalls atomic.Int64
	spendLimitCalls          atomic.Int64
	spendLimitSource         string
	spendLimitAmount         float64
}

func (s *billingEligibilityResolverStub) Resolve(context.Context, int64) (*BillingContext, error) {
	s.calls.Add(1)
	return s.result, s.err
}

func (s *billingEligibilityResolverStub) ResolveForAmount(context.Context, int64, float64) (*BillingContext, error) {
	s.calls.Add(1)
	return s.result, s.err
}

func (s *billingEligibilityResolverStub) GetOrganizationBalance(context.Context, *BillingContext) (float64, error) {
	s.organizationBalanceCalls.Add(1)
	return s.organizationBalance, s.organizationBalanceErr
}

func (s *billingEligibilityResolverStub) CheckSpendLimit(_ context.Context, billing *BillingContext, amount float64) error {
	s.spendLimitCalls.Add(1)
	s.spendLimitSource = billing.BalanceSource
	s.spendLimitAmount = amount
	return s.spendLimitErr
}

func (s *balanceEligibilityCacheStub) DeductUserBalance(context.Context, int64, float64) error {
	s.deductCalls.Add(1)
	return nil
}

func (s *balanceEligibilityCacheStub) InvalidateUserBalance(context.Context, int64) error {
	s.invalidateCalls.Add(1)
	s.invalidated.Store(true)
	return nil
}

func TestCheckBillingEligibility_RejectsBalanceBelowMinimumReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.005}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestCheckBillingEligibility_AllowsBalanceAtMinimumReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.01}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
}

func TestCheckBillingEligibility_CompanyBalanceDoesNotReadOwnerCache(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1}
	organizationID := int64(8)
	resolver := &billingEligibilityResolverStub{result: &BillingContext{
		ConsumerUserID: 20, OrganizationID: &organizationID,
		PayerUserID: 10, BalanceSource: BalanceSourceCompany,
	}, organizationBalance: 1}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)

	require.NoError(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, nil, nil, nil, ""))
	require.Zero(t, cache.lastUserID.Load())
	require.Equal(t, int64(1), resolver.calls.Load())
	require.Equal(t, int64(1), resolver.organizationBalanceCalls.Load())
}

func TestSyncBalanceCacheAfterDeduction_CompanyBalanceSkipsOwnerCache(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 100}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)
	newBalance := 8.0

	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 2}, User: &User{ID: 20},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{
		NewBalance: &newBalance, PayerUserID: 10, BalanceSource: BalanceSourceCompany,
	})

	require.Zero(t, cache.invalidateCalls.Load())
	require.Zero(t, cache.deductCalls.Load())
}

func TestCheckBillingEligibility_AllocatedBalanceChecksMemberPayer(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1}
	resolver := &billingEligibilityResolverStub{result: &BillingContext{
		ConsumerUserID: 20,
		PayerUserID:    20,
		BalanceSource:  "allocated",
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)

	require.NoError(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, nil, nil, nil, ""))
	require.Equal(t, int64(20), cache.lastUserID.Load())
}

func TestCheckBillingEligibility_StandardRequestRejectsExhaustedOrganizationLimitBeforeAllocationFallback(t *testing.T) {
	resolver := &billingEligibilityResolverStub{
		result: &BillingContext{
			ConsumerUserID: 20,
			PayerUserID:    20,
			BalanceSource:  BalanceSourceAllocated,
		},
		spendLimitErr: ErrSpendLimitExceeded,
	}
	svc := NewBillingCacheService(&balanceEligibilityCacheStub{balance: 0.003}, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, nil, nil, nil, "openai")
	require.ErrorIs(t, err, ErrSpendLimitExceeded)
	require.Zero(t, resolver.calls.Load())
	require.Equal(t, int64(1), resolver.spendLimitCalls.Load())
	require.Equal(t, BalanceSourceCompany, resolver.spendLimitSource)
	require.Zero(t, resolver.spendLimitAmount)
}

func TestCheckBillingEligibility_EnterpriseSubscriptionChecksSpendLimitWithoutAllocatedBalance(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0}
	resolver := &billingEligibilityResolverStub{result: &BillingContext{
		ConsumerUserID: 20,
		PayerUserID:    20,
		BalanceSource:  "allocated",
	}}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)
	organizationSubscriptionID := int64(501)
	group := &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription}
	key := &APIKey{OrganizationSubscriptionID: &organizationSubscriptionID, Group: group}

	require.NoError(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, key, group, nil, "openai"))
	require.NoError(t, svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, key, nil, nil, "openai"))
	require.Zero(t, cache.lastUserID.Load())
	require.Zero(t, resolver.calls.Load())
	require.Equal(t, int64(2), resolver.spendLimitCalls.Load())
	require.Equal(t, BalanceSourceSubscription, resolver.spendLimitSource)
	require.Zero(t, resolver.spendLimitAmount)
}

func TestCheckBillingEligibility_EnterpriseSubscriptionRejectsExceededMemberLimit(t *testing.T) {
	resolver := &billingEligibilityResolverStub{spendLimitErr: ErrSpendLimitExceeded}
	svc := NewBillingCacheService(&balanceEligibilityCacheStub{}, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)
	organizationSubscriptionID := int64(501)
	key := &APIKey{OrganizationSubscriptionID: &organizationSubscriptionID}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, key, nil, nil, "openai")
	require.ErrorIs(t, err, ErrSpendLimitExceeded)
	require.Equal(t, int64(1), resolver.spendLimitCalls.Load())
}

func TestCheckBillingEligibility_PersonalSubscriptionRejectsExceededOrganizationMemberLimit(t *testing.T) {
	resolver := &billingEligibilityResolverStub{spendLimitErr: ErrSpendLimitExceeded}
	svc := NewBillingCacheService(&balanceEligibilityCacheStub{}, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)
	group := &Group{ID: 88, SubscriptionType: SubscriptionTypeSubscription}
	subscription := &UserSubscription{Status: SubscriptionStatusActive}

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, nil, group, subscription, "openai")
	require.ErrorIs(t, err, ErrSpendLimitExceeded)
	require.Equal(t, int64(1), resolver.spendLimitCalls.Load())
	require.Equal(t, BalanceSourceSubscription, resolver.spendLimitSource)
	require.Zero(t, resolver.spendLimitAmount)
}

func TestCheckBillingEligibility_PayerResolutionFailsClosed(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1}
	expected := errors.New("payer resolution unavailable")
	resolver := &billingEligibilityResolverStub{err: expected}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingContextResolver(resolver)
	t.Cleanup(svc.Stop)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 20}, nil, nil, nil, "")
	require.ErrorIs(t, err, expected)
	require.Zero(t, cache.lastUserID.Load())
}

func TestSyncBalanceCacheAfterDeduction_InvalidatesExhaustedBalance(t *testing.T) {
	cache := &balanceEligibilityCacheStub{
		balance:                  0.50,
		cacheMissAfterInvalidate: true,
	}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := -0.25
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.75},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{
		NewBalance:         &newBalance,
		BalanceOverdrafted: true,
	})

	require.Equal(t, int64(1), cache.invalidateCalls.Load())
	require.Equal(t, int64(0), cache.deductCalls.Load())

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}

func TestSyncBalanceCacheAfterDeduction_InvalidatesWhenBalanceFallsBelowReserve(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 0.50}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := 0.005
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.495},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{NewBalance: &newBalance})

	require.Equal(t, int64(1), cache.invalidateCalls.Load())
	require.Equal(t, int64(0), cache.deductCalls.Load())
}

func TestSyncBalanceCacheAfterDeduction_QueuesDeductWhenBalanceStillEligible(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1}
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.01
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	newBalance := 0.75
	syncBalanceCacheAfterDeduction(context.Background(), &postUsageBillingParams{
		Cost: &CostBreakdown{ActualCost: 0.25},
		User: &User{ID: 1},
	}, &billingDeps{billingCacheService: svc}, &UsageBillingApplyResult{NewBalance: &newBalance})

	require.Equal(t, int64(0), cache.invalidateCalls.Load())
	require.Eventually(t, func() bool {
		return cache.deductCalls.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}
