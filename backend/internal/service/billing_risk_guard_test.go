//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type billingRiskStoreStub struct {
	acquireResult           *BillingRiskAcquireResult
	acquireErr              error
	acquireCalls            int
	lastAcquire             BillingRiskAcquireRequest
	refreshCalls            int
	commitCalls             int
	releaseCalls            int
	uncertainCalls          int
	lastBalance             int64
	resetCalls              int
	lastResetBalance        int64
	lastResetVersion        int64
	resetResult             *BillingRiskBalanceResetResult
	resetErr                error
	balanceVersion          int64
	balanceVersionErr       error
	lastUncertainContextErr error
	refreshLost             bool
	commitHook              func()
}

func (s *billingRiskStoreStub) Acquire(_ context.Context, request BillingRiskAcquireRequest) (*BillingRiskAcquireResult, error) {
	s.acquireCalls++
	s.lastAcquire = request
	if s.acquireResult == nil && s.acquireErr == nil {
		return &BillingRiskAcquireResult{Acquired: true}, nil
	}
	return s.acquireResult, s.acquireErr
}

func (s *billingRiskStoreStub) Refresh(_ context.Context, _ int64, _ string, _, _ time.Duration) (bool, error) {
	s.refreshCalls++
	return !s.refreshLost, nil
}

func (s *billingRiskStoreStub) Commit(_ context.Context, _ int64, _ string, balance int64, _ time.Duration) (bool, error) {
	s.commitCalls++
	s.lastBalance = balance
	if s.commitHook != nil {
		s.commitHook()
	}
	return true, nil
}

func (s *billingRiskStoreStub) Release(_ context.Context, _ int64, _ string, _ time.Duration) (bool, error) {
	s.releaseCalls++
	return true, nil
}

func (s *billingRiskStoreStub) MarkUncertain(ctx context.Context, _ int64, _ string, _ int64, _, _ time.Duration) (bool, error) {
	s.uncertainCalls++
	s.lastUncertainContextErr = ctx.Err()
	if s.lastUncertainContextErr != nil {
		return false, s.lastUncertainContextErr
	}
	return true, nil
}

func (s *billingRiskStoreStub) GetBalanceVersion(context.Context, int64) (int64, error) {
	return s.balanceVersion, s.balanceVersionErr
}

func (s *billingRiskStoreStub) ResetBalance(_ context.Context, _ int64, balance, version int64, _ time.Duration) (*BillingRiskBalanceResetResult, error) {
	s.resetCalls++
	s.lastResetBalance = balance
	s.lastResetVersion = version
	if s.resetResult != nil || s.resetErr != nil {
		return s.resetResult, s.resetErr
	}
	return &BillingRiskBalanceResetResult{Accepted: true, KnownBalanceMicros: balance}, nil
}

func newBillingRiskGuardTest(t *testing.T, enabled bool, store *billingRiskStoreStub) *BillingRiskGuard {
	t.Helper()
	settings := NewSettingService(nil, nil)
	configured := DefaultBillingRiskSettings()
	configured.Enabled = enabled
	settings.storeBillingRiskSettings(configured)
	return NewBillingRiskGuard(store, settings)
}

func TestBillingRiskGuardBypassesOffSubscriptionAndHighBalanceText(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		request BillingRiskRequest
	}{
		{name: "总开关关闭", enabled: false, request: BillingRiskRequest{UserID: 1, Balance: 1, Kind: BillingRiskRequestText, EstimatedCost: 1, EstimateCertain: true}},
		{name: "订阅计费", enabled: true, request: BillingRiskRequest{UserID: 1, Balance: 1, SubscriptionBilling: true, Kind: BillingRiskRequestText}},
		{name: "高余额文本", enabled: true, request: BillingRiskRequest{UserID: 1, Balance: 20, Kind: BillingRiskRequestText, EstimatedCost: 100, EstimateCertain: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &billingRiskStoreStub{}
			guard := newBillingRiskGuardTest(t, tt.enabled, store)
			permit, err := guard.Acquire(context.Background(), tt.request)
			require.NoError(t, err)
			require.Nil(t, permit)
			require.Zero(t, store.acquireCalls)
		})
	}
}

func TestBillingRiskGuardLowBalanceUsesWeightedRiskInIntegerMicros(t *testing.T) {
	store := &billingRiskStoreStub{}
	guard := newBillingRiskGuardTest(t, true, store)

	permit, err := guard.Acquire(context.Background(), BillingRiskRequest{
		UserID:                2,
		Balance:               1,
		MinimumBalanceReserve: 0.000001,
		Kind:                  BillingRiskRequestText,
		EstimatedCost:         0.01,
		EstimateCertain:       true,
	})

	require.NoError(t, err)
	require.NotNil(t, permit)
	require.Equal(t, int64(12_500), store.lastAcquire.RiskMicros)
	require.Equal(t, int64(1_000_000), store.lastAcquire.BalanceMicros)
	require.Equal(t, int64(1), store.lastAcquire.MinimumReserveMicros)
	require.Equal(t, int64(200_000), store.lastAcquire.OverdraftAllowanceMicros)
}

func TestBillingRiskGuardMinimumRiskAndConservativeRoundingNeverUnderReserve(t *testing.T) {
	store := &billingRiskStoreStub{}
	guard := newBillingRiskGuardTest(t, true, store)

	_, err := guard.Acquire(context.Background(), BillingRiskRequest{
		UserID:          3,
		Balance:         0.9999999,
		Kind:            BillingRiskRequestText,
		EstimatedCost:   0.0000001,
		EstimateCertain: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1_000), store.lastAcquire.RiskMicros)
	require.Equal(t, int64(999_999), store.lastAcquire.BalanceMicros)
}

func TestBillingRiskGuardZeroConfiguredMinimumStillUsesOneMicro(t *testing.T) {
	store := &billingRiskStoreStub{}
	settings := NewSettingService(nil, nil)
	configured := DefaultBillingRiskSettings()
	configured.Enabled = true
	configured.MinimumRequestRisk = 0
	configured.OverdraftAllowance = 0
	settings.storeBillingRiskSettings(configured)
	guard := NewBillingRiskGuard(store, settings)

	_, err := guard.Acquire(context.Background(), BillingRiskRequest{
		UserID:          31,
		Balance:         1,
		Kind:            BillingRiskRequestText,
		EstimatedCost:   0,
		EstimateCertain: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), store.lastAcquire.RiskMicros)
}

func TestBillingRiskGuardProtectsHighCostMediaAndAllVideoAboveBalanceThreshold(t *testing.T) {
	tests := []BillingRiskRequest{
		{UserID: 4, Balance: 100, Kind: BillingRiskRequestSyncImage, EstimatedCost: 1, EstimateCertain: true},
		{UserID: 4, Balance: 100, Kind: BillingRiskRequestVideo, EstimatedCost: 0.01, EstimateCertain: true},
	}
	for _, request := range tests {
		store := &billingRiskStoreStub{}
		guard := newBillingRiskGuardTest(t, true, store)
		permit, err := guard.Acquire(context.Background(), request)
		require.NoError(t, err)
		require.NotNil(t, permit)
		require.Equal(t, 1, store.acquireCalls)
	}
}

func TestBillingRiskGuardUnknownEstimateUsesExclusiveAvailableBudget(t *testing.T) {
	store := &billingRiskStoreStub{}
	guard := newBillingRiskGuardTest(t, true, store)

	_, err := guard.Acquire(context.Background(), BillingRiskRequest{
		UserID:                5,
		Balance:               5,
		MinimumBalanceReserve: 0.1,
		Kind:                  BillingRiskRequestSyncImage,
		EstimateCertain:       false,
	})

	require.NoError(t, err)
	require.Equal(t, int64(5_100_000), store.lastAcquire.RiskMicros)
}

func TestBillingRiskGuardFailsClosedOnRedisError(t *testing.T) {
	redisErr := errors.New("redis unavailable")

	store := &billingRiskStoreStub{acquireErr: redisErr}
	guard := newBillingRiskGuardTest(t, true, store)
	permit, err := guard.Acquire(context.Background(), BillingRiskRequest{UserID: 6, Balance: 1, Kind: BillingRiskRequestText, EstimatedCost: 0.1, EstimateCertain: true})
	require.Nil(t, permit)
	require.Error(t, err)
	require.Equal(t, "BILLING_RISK_GUARD_UNAVAILABLE", infraerrors.Reason(err))
}

func TestBillingRiskGuardRejectsBudgetAndDelegatesPermitLifecycle(t *testing.T) {
	store := &billingRiskStoreStub{acquireResult: &BillingRiskAcquireResult{Acquired: false, WouldReject: true}}
	guard := newBillingRiskGuardTest(t, true, store)
	permit, err := guard.Acquire(context.Background(), BillingRiskRequest{UserID: 7, Balance: 1, Kind: BillingRiskRequestText, EstimatedCost: 1, EstimateCertain: true})
	require.Nil(t, permit)
	require.Equal(t, "BILLING_RISK_BUDGET_EXCEEDED", infraerrors.Reason(err))

	store.acquireResult = &BillingRiskAcquireResult{Acquired: true}
	permit, err = guard.Acquire(context.Background(), BillingRiskRequest{UserID: 7, Balance: 1, Kind: BillingRiskRequestText, EstimatedCost: 1, EstimateCertain: true})
	require.NoError(t, err)
	require.NotNil(t, permit)

	require.NoError(t, guard.Refresh(context.Background(), permit))
	require.NoError(t, guard.Commit(context.Background(), permit, 0.4))
	require.NoError(t, guard.Release(context.Background(), permit))
	require.NoError(t, guard.MarkUncertain(context.Background(), permit))
	require.Equal(t, 1, store.refreshCalls)
	require.Equal(t, 1, store.commitCalls)
	require.Equal(t, int64(400_000), store.lastBalance)
	require.Equal(t, 1, store.releaseCalls)
	require.Equal(t, 1, store.uncertainCalls)
}

func TestBillingRiskGuardRefreshReportsLostLease(t *testing.T) {
	store := &billingRiskStoreStub{refreshLost: true}
	guard := newBillingRiskGuardTest(t, true, store)
	permit := &BillingRiskPermit{
		UserID:            7,
		LeaseID:           "lost-lease",
		RiskMicros:        800_000,
		LeaseTTL:          time.Minute,
		IdleTTL:           2 * time.Minute,
		UncertainCooldown: 5 * time.Minute,
		guard:             guard,
	}

	err := guard.Refresh(context.Background(), permit)

	require.Error(t, err)
	require.Contains(t, err.Error(), "lease")
}
