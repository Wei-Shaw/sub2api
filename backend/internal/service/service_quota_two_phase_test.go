//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// shared mocks live in service_quota_fakes_test.go.

// matchesGlobPattern is the minimal subset of SCAN MATCH glob: only `*`
// (no `?` or `[]`) is supported, sufficient for the service-quota patterns
// (svcquota:v2:<rule>:* or svcquota:v2:<rule>:*:<type>:*). Used by
// fakeServiceQuotaLimiter.ResetPattern.
func matchesGlobPattern(s, pattern string) bool {
	if pattern == "" {
		return false
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return s == pattern
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	cur := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s[cur:], parts[i])
		if idx < 0 {
			return false
		}
		cur += idx + len(parts[i])
	}
	last := parts[len(parts)-1]
	return strings.HasSuffix(s[cur:], last)
}

// newQuotaServiceForTest assembles a *serviceQuotaService with the minimum
// dependencies needed for two-phase precheck tests. settingRepo is caller-owned
// so feature flags can be flipped freely.
func newQuotaServiceForTest(t *testing.T, rules []*ServiceQuotaRule, settingRepo *mockSettingRepo, limiter *fakeServiceQuotaLimiter) *serviceQuotaService {
	t.Helper()
	if settingRepo == nil {
		settingRepo = newMockSettingRepo()
	}
	settingRepo.data[SettingKeyServiceQuotaEnabled] = "true"
	settings := NewSettingService(settingRepo, &config.Config{})
	repo := &fakeServiceQuotaRepo{rules: rules}
	return &serviceQuotaService{
		repo:     repo,
		settings: settings,
		limiter:  limiter,
	}
}

func ruleConcurrencyAccount(id, accountID int64, limit float64) *ServiceQuotaRule {
	acc := accountID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterConcurrency, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc},
		},
	}
}

func ruleRPMChannel(id, channelID int64, limit float64) *ServiceQuotaRule {
	ch := channelID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: true},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, ChannelID: &ch},
		},
	}
}

func TestPreCheckSelect_ReturnsCandidatesWithoutAcquire(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(1, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42, Platform: "anthropic"})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.Rules, 1)
	require.Equal(t, int64(1), plan.Rules[0].ID)
	require.False(t, plan.PreparedAt.IsZero())

	require.Empty(t, limiter.acquireCalls)
	require.Empty(t, limiter.incrementCalls)
}

func TestPreCheckAcquire_ChannelScopeRPMHitsOnlyMatchingChannel(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMChannel(2, 99, 100)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 0)
	require.NoError(t, err)
	require.Nil(t, lease)
	require.Empty(t, limiter.incrementCalls, "channel=0 must not trigger RPM Increment")

	lease, err = svc.PreCheckAcquire(context.Background(), plan, 99, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Nil(t, lease.Release)
	require.Len(t, limiter.incrementCalls, 1)
}

func TestPreCheckAcquire_AccountConcurrencyEnforced(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(3, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	lease1, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.NoError(t, err)
	require.NotNil(t, lease1)
	require.NotNil(t, lease1.Release)

	lease2, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.Error(t, err)
	require.Nil(t, lease2)
	require.True(t, errors.Is(err, ErrServiceQuotaExceeded) || err.Error() != "")

	lease1.Release()
	lease3, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.NoError(t, err)
	require.NotNil(t, lease3)
	lease3.Release()
}

func TestPreCheckAcquire_DifferentAccountSamRulePathMissesBecauseOfPathFilter(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(4, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)

	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 8)
	require.NoError(t, err)
	require.Nil(t, lease)
	require.Empty(t, limiter.acquireCalls)
}

func TestIsPreCheckTwoPhase_DefaultFalse(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	repo := newMockSettingRepo()
	svc := newQuotaServiceForTest(t, nil, repo, limiter)
	require.False(t, svc.IsPreCheckTwoPhase(context.Background()))
}

func TestIsPreCheckTwoPhase_ReadsSetting(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	repo := newMockSettingRepo()
	repo.data[SettingKeyServiceQuotaPreCheckTwoPhase] = "true"
	svc := newQuotaServiceForTest(t, nil, repo, limiter)
	require.True(t, svc.IsPreCheckTwoPhase(context.Background()))
}

func TestPreCheck_LegacySinglePhase_StillWorks(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(5, 7, 1)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	lease, err := svc.PreCheck(context.Background(), ServiceQuotaCheckRequest{UserID: 42, AccountID: 7})
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Len(t, limiter.acquireCalls, 1)
	lease.Release()
}

func TestBillingTicket_Consume_FlagOff_NoOp(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(6, 7, 5)
	repo := newMockSettingRepo()
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, repo, limiter)

	ticket := &BillingTicket{
		svc:          nil,
		quotaReq:     ServiceQuotaCheckRequest{UserID: 42, AccountID: 7},
		twoPhase:     false,
		legacyOneOff: true,
	}
	lease, err := svc.PreCheck(context.Background(), ServiceQuotaCheckRequest{UserID: 42, AccountID: 7})
	require.NoError(t, err)
	require.NotNil(t, lease)
	ticket.lease = wrapServiceQuotaLeaseOnce(lease)

	acquireCallsBefore := len(limiter.acquireCalls)
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Equal(t, acquireCallsBefore, len(limiter.acquireCalls), "Consume with flag off must not Acquire again")

	ticket.Close()
	require.Error(t, ticket.Consume(context.Background(), 0, 7))
}

func TestBillingTicket_Consume_FlagOn_AcquiresLazily(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(7, 7, 5)
	repo := newMockSettingRepo()
	repo.data[SettingKeyServiceQuotaPreCheckTwoPhase] = "true"
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, repo, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: svc},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     plan,
		twoPhase: true,
	}

	require.Empty(t, limiter.acquireCalls, "Prepare phase must not Acquire")
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Len(t, limiter.acquireCalls, 1, "Consume phase must trigger Acquire")
	require.NotNil(t, ticket.lease)

	require.NoError(t, ticket.Consume(context.Background(), 0, 8))
	require.Nil(t, ticket.lease, "swapping to non-matching account must clear the lease")

	ticket.Close()
}
