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
}

type billingRiskBalanceVersionStoreStub struct {
	BillingRiskStore
	version int64
	calls   atomic.Int64
}

type advancingBillingRiskBalanceVersionStoreStub struct {
	BillingRiskStore
	calls atomic.Int64
}

func (s *billingRiskBalanceVersionStoreStub) GetBalanceVersion(context.Context, int64) (int64, error) {
	s.calls.Add(1)
	return s.version, nil
}

func (s *advancingBillingRiskBalanceVersionStoreStub) GetBalanceVersion(context.Context, int64) (int64, error) {
	if s.calls.Add(1) == 1 {
		return 7, nil
	}
	return 8, nil
}

func (s *balanceEligibilityCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	if s.cacheMissAfterInvalidate && s.invalidated.Load() {
		return 0, errors.New("cache miss")
	}
	return s.balance, nil
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

func TestCheckBillingEligibilityWithBalanceReturnsCurrentBillingBalance(t *testing.T) {
	cache := &balanceEligibilityCacheStub{balance: 1.25}
	cfg := &config.Config{}
	svc := NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)

	user := &User{ID: 1, Balance: 20}
	balance, err := svc.CheckBillingEligibilityWithBalance(context.Background(), user, nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, 1.25, balance)
	require.Equal(t, 1.25, user.Balance)
	require.False(t, user.BillingBalanceAuthoritative)
}

func TestCheckBillingEligibilityMarksDatabaseBalanceAsAuthoritativeAfterCacheMiss(t *testing.T) {
	cache := &balanceEligibilityCacheStub{cacheMissAfterInvalidate: true}
	cache.invalidated.Store(true)
	userRepo := &balanceLoadUserRepoStub{balance: 0.75}
	riskStore := &billingRiskBalanceVersionStoreStub{version: 7}
	svc := NewBillingCacheService(cache, userRepo, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingRiskStore(riskStore)
	t.Cleanup(svc.Stop)

	user := &User{ID: 1, Balance: 20}
	balance, err := svc.CheckBillingEligibilityWithBalance(context.Background(), user, nil, nil, nil, "")

	require.NoError(t, err)
	require.Equal(t, 0.75, balance)
	require.Equal(t, 0.75, user.Balance)
	require.True(t, user.BillingBalanceAuthoritative)
	require.Equal(t, int64(7), user.BillingBalanceVersion)
	require.Equal(t, int64(1), riskStore.calls.Load())
}

func TestBillingCacheServiceSkipsStaleDatabaseBalanceWriteAfterRiskVersionChanges(t *testing.T) {
	cache := &billingCacheMissStub{}
	userRepo := &balanceLoadUserRepoStub{balance: 20}
	riskStore := &advancingBillingRiskBalanceVersionStoreStub{}
	svc := NewBillingCacheService(cache, userRepo, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetBillingRiskStore(riskStore)
	t.Cleanup(svc.Stop)

	snapshot, err := svc.GetUserBalanceSnapshot(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, 20.0, snapshot.Balance)
	require.Equal(t, int64(7), snapshot.RiskVersion)
	require.Eventually(t, func() bool {
		return riskStore.calls.Load() >= 2 || cache.setBalanceCalls.Load() > 0
	}, time.Second, 5*time.Millisecond)
	require.Equal(t, int64(2), riskStore.calls.Load(), "后台写入前必须复核风险余额版本")
	require.Zero(t, cache.setBalanceCalls.Load(), "旧 DB 余额不得覆盖较新的风险余额")
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
