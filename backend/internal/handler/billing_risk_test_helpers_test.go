package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type handlerBillingRiskStoreStub struct {
	mu             sync.Mutex
	refreshCalls   int
	releaseCalls   int
	uncertainCalls int
	refreshLost    bool
}

type handlerBillingRiskBudgetStore struct {
	mu       sync.Mutex
	reserved int64
	leases   map[string]int64
	version  int64
}

func (s *handlerBillingRiskBudgetStore) Acquire(_ context.Context, request service.BillingRiskAcquireRequest) (*service.BillingRiskAcquireResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leases == nil {
		s.leases = make(map[string]int64)
	}
	if _, exists := s.leases[request.LeaseID]; exists {
		return &service.BillingRiskAcquireResult{Acquired: true, ReservedTotalMicros: s.reserved}, nil
	}
	budget := request.BalanceMicros - request.MinimumReserveMicros + request.OverdraftAllowanceMicros
	if s.reserved+request.RiskMicros > budget {
		return &service.BillingRiskAcquireResult{WouldReject: true}, nil
	}
	s.leases[request.LeaseID] = request.RiskMicros
	s.reserved += request.RiskMicros
	return &service.BillingRiskAcquireResult{Acquired: true, ReservedTotalMicros: s.reserved}, nil
}

func (s *handlerBillingRiskBudgetStore) Refresh(context.Context, int64, string, time.Duration, time.Duration) (bool, error) {
	return true, nil
}

func (s *handlerBillingRiskBudgetStore) Commit(_ context.Context, _ int64, leaseID string, _ int64, _ time.Duration) (bool, error) {
	return s.remove(leaseID), nil
}

func (s *handlerBillingRiskBudgetStore) Release(_ context.Context, _ int64, leaseID string, _ time.Duration) (bool, error) {
	return s.remove(leaseID), nil
}

func (s *handlerBillingRiskBudgetStore) MarkUncertain(context.Context, int64, string, int64, time.Duration, time.Duration) (bool, error) {
	return true, nil
}

func (s *handlerBillingRiskBudgetStore) GetBalanceVersion(context.Context, int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version, nil
}

func (s *handlerBillingRiskBudgetStore) ResetBalance(_ context.Context, _ int64, balance, _ int64, _ time.Duration) (*service.BillingRiskBalanceResetResult, error) {
	return &service.BillingRiskBalanceResetResult{Accepted: true, KnownBalanceMicros: balance}, nil
}

func (s *handlerBillingRiskBudgetStore) remove(leaseID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	risk, exists := s.leases[leaseID]
	if !exists {
		return false
	}
	delete(s.leases, leaseID)
	s.reserved -= risk
	return true
}

func (s *handlerBillingRiskBudgetStore) leaseCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.leases)
}

func (s *handlerBillingRiskStoreStub) Acquire(context.Context, service.BillingRiskAcquireRequest) (*service.BillingRiskAcquireResult, error) {
	return &service.BillingRiskAcquireResult{Acquired: true}, nil
}

func (s *handlerBillingRiskStoreStub) Refresh(context.Context, int64, string, time.Duration, time.Duration) (bool, error) {
	s.mu.Lock()
	s.refreshCalls++
	lost := s.refreshLost
	s.mu.Unlock()
	return !lost, nil
}

func (s *handlerBillingRiskStoreStub) Commit(context.Context, int64, string, int64, time.Duration) (bool, error) {
	return true, nil
}

func (s *handlerBillingRiskStoreStub) Release(context.Context, int64, string, time.Duration) (bool, error) {
	s.mu.Lock()
	s.releaseCalls++
	s.mu.Unlock()
	return true, nil
}

func (s *handlerBillingRiskStoreStub) MarkUncertain(context.Context, int64, string, int64, time.Duration, time.Duration) (bool, error) {
	s.mu.Lock()
	s.uncertainCalls++
	s.mu.Unlock()
	return true, nil
}

func (s *handlerBillingRiskStoreStub) GetBalanceVersion(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *handlerBillingRiskStoreStub) ResetBalance(_ context.Context, _ int64, balance, _ int64, _ time.Duration) (*service.BillingRiskBalanceResetResult, error) {
	return &service.BillingRiskBalanceResetResult{Accepted: true, KnownBalanceMicros: balance}, nil
}

func (s *handlerBillingRiskStoreStub) counts() (refresh, release, uncertain int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshCalls, s.releaseCalls, s.uncertainCalls
}

type handlerBillingRiskSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
}

func (s *handlerBillingRiskSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func newHandlerBillingRiskPermit(t *testing.T, store service.BillingRiskStore) *service.BillingRiskPermit {
	t.Helper()
	settings := newEnabledBillingRiskSettingService(t)
	guard := service.NewBillingRiskGuard(store, settings)
	permit, err := guard.Acquire(context.Background(), service.BillingRiskRequest{
		UserID:          1,
		Balance:         1,
		Kind:            service.BillingRiskRequestText,
		EstimatedCost:   0.1,
		EstimateCertain: true,
	})
	require.NoError(t, err)
	require.NotNil(t, permit)
	permit.RefreshInterval = 5 * time.Millisecond
	return permit
}

func newEnabledBillingRiskSettingService(t *testing.T) *service.SettingService {
	t.Helper()
	repo := &handlerBillingRiskSettingRepoStub{values: map[string]string{
		service.SettingKeyBillingRiskEnabled: "true",
	}}
	settings := service.NewSettingService(repo, &config.Config{})
	require.NoError(t, settings.LoadBillingRiskSettings(context.Background()))
	return settings
}
