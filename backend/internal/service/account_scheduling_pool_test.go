package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func schedulingTestCandidates(count int) []AccountSchedulingCandidate {
	result := make([]AccountSchedulingCandidate, 0, count)
	for i := 1; i <= count; i++ {
		result = append(result, AccountSchedulingCandidate{Identity: fmt.Sprint(i), AccountID: int64(i), Position: int64(i), ExitGroup: "shared", State: AccountSchedulingStandby, Reason: "ready"})
	}
	return result
}

func TestAccountSchedulingFixedSetDrainsAndRecoversToTail(t *testing.T) {
	now := time.Now()
	candidates := schedulingTestCandidates(5)
	slots, allowed := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, now)
	require.True(t, allowed[1])
	require.True(t, allowed[2])
	require.True(t, allowed[3])
	require.False(t, allowed[4])
	candidates[0].State, candidates[0].Reason = AccountSchedulingCooldown, "quota_exhausted"
	slots, allowed = ReconcileAccountSchedulingSlots(candidates, slots, map[string]int{"1": 1}, 3, now.Add(time.Second))
	require.False(t, allowed[1])
	require.False(t, allowed[4], "an open stream still occupies the exit's third identity")
	slots, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(2*time.Second))
	require.True(t, allowed[4])
	candidates[0].State, candidates[0].Reason = AccountSchedulingStandby, "ready"
	slots, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(3*time.Second))
	require.False(t, allowed[1], "quota recovery must not preempt a current active account")
	candidates[1].State, candidates[1].Reason = AccountSchedulingCooldown, "quota_exhausted"
	_, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(4*time.Second))
	require.True(t, allowed[5], "existing standby precedes a recovered identity")
	require.False(t, allowed[1])
}

func TestAccountSchedulingIdentityDedupAndSharedExit(t *testing.T) {
	candidates := schedulingTestCandidates(6)
	candidates[1].Identity = candidates[0].Identity
	candidates[4].ExitGroup, candidates[5].ExitGroup = "other", "other"
	_, allowed := ReconcileAccountSchedulingSlots(candidates, nil, nil, 2, time.Now())
	require.True(t, allowed[1])
	require.True(t, allowed[2])
	require.True(t, allowed[3])
	require.False(t, allowed[4])
	require.True(t, allowed[5])
	require.True(t, allowed[6])
	candidates[1].ExitGroup = "other"
	_, allowed = ReconcileAccountSchedulingSlots(candidates, nil, nil, 2, time.Now())
	require.False(t, allowed[1])
	require.False(t, allowed[2], "same identity cannot silently move between independently budgeted exits")
}

func TestAccountSchedulingTransientCooldownDoesNotCycleReserve(t *testing.T) {
	now := time.Now()
	candidates := schedulingTestCandidates(4)
	slots, _ := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, now)
	candidates[0].State, candidates[0].Reason = AccountSchedulingCooldown, "temporary_cooldown"
	slots, allowed := ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(time.Second))
	require.False(t, allowed[1])
	require.False(t, allowed[4])
	candidates[0].State, candidates[0].Reason = AccountSchedulingStandby, "ready"
	_, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(2*time.Second))
	require.True(t, allowed[1])
	require.False(t, allowed[4])
}

func TestAccountSchedulingLimitReductionCountsLateDrainingSlot(t *testing.T) {
	now := time.Now()
	candidates := schedulingTestCandidates(4)
	slots, _ := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, now)
	candidates[2].State, candidates[2].Reason = AccountSchedulingCooldown, "quota_exhausted"
	slots, allowed := ReconcileAccountSchedulingSlots(candidates, slots, map[string]int{"3": 1}, 1, now.Add(time.Second))
	for _, isAllowed := range allowed {
		require.False(t, isAllowed)
	}
	require.Equal(t, AccountSchedulingDraining, slots[2].State)
}

func TestAccountSchedulingPermanentAuthBurstStopsReplacement(t *testing.T) {
	now := time.Now()
	candidates := schedulingTestCandidates(6)
	slots, _ := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, now)
	for i := 0; i < 3; i++ {
		candidates[i].State, candidates[i].Reason = AccountSchedulingReauth, "authentication_failed"
	}
	slots, allowed := ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(time.Second))
	for _, isAllowed := range allowed {
		require.False(t, isAllowed)
	}
	for _, slot := range slots {
		if slot.AccountID > 3 {
			require.Equal(t, "exit_auth_circuit_open", slot.Reason)
		}
	}
	_, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, now.Add(11*time.Minute))
	require.True(t, allowed[4])
	require.True(t, allowed[5])
	require.True(t, allowed[6])
}

func TestAccountSchedulingExistingAuthErrorsDoNotTripCircuit(t *testing.T) {
	candidates := schedulingTestCandidates(6)
	for i := 0; i < 3; i++ {
		candidates[i].State, candidates[i].Reason = AccountSchedulingReauth, "authentication_failed"
	}
	slots, allowed := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, time.Now())
	require.True(t, allowed[4])
	require.True(t, allowed[5])
	require.True(t, allowed[6])
	_, allowed = ReconcileAccountSchedulingSlots(candidates, slots, nil, 3, time.Now())
	require.True(t, allowed[4])
}

func TestAccountSchedulingActualExitRequiresFreshRoute(t *testing.T) {
	proxyID := int64(9)
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, ProxyID: &proxyID, Credentials: map[string]any{"plan_type": "free", "chatgpt_account_id": "stable"}}
	proxy := Proxy{ID: 9, Protocol: "http", Host: "localhost", Port: 3128}
	policy := AccountSchedulingPolicy{Enabled: true, MaxActiveAccounts: 3, ProxyExitGroups: map[string]string{"9": "shared"}}
	_, err := BuildAccountSchedulingCandidates(policy, []Account{account}, nil, nil, []Proxy{proxy}, 1, "", time.Now())
	require.ErrorIs(t, err, ErrAccountSchedulingUnavailable, "stale account hydration must not silently use direct egress")
	candidates, err := BuildAccountSchedulingCandidates(policy, []Account{account}, nil, nil, []Proxy{proxy}, 1, proxy.URL(), time.Now())
	require.NoError(t, err)
	require.Equal(t, "shared", candidates[0].ExitGroup)
	duplicate := proxy
	duplicate.ID = 10
	policy.ProxyExitGroups["10"] = "other"
	_, err = BuildAccountSchedulingCandidates(policy, []Account{account}, nil, nil, []Proxy{duplicate, proxy}, 1, proxy.URL(), time.Now())
	require.Error(t, err, "conflicting proxy aliases must fail independent of query order")
}

func TestAccountSchedulingUsesActualQuotaResetAndKeepsManualDisable(t *testing.T) {
	now := time.Now()
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"plan_type": "free", "email": "test@example.invalid"}, Extra: map[string]any{"codex_7d_used_percent": 100.0, "codex_7d_window_minutes": 43200, "codex_7d_reset_at": now.Add(20 * 24 * time.Hour).Format(time.RFC3339), "codex_usage_updated_at": now.Format(time.RFC3339)}}
	state, reason := accountSchedulingCandidateState(&account, now)
	require.Equal(t, AccountSchedulingCooldown, state)
	require.Equal(t, "quota_exhausted", reason)
	account.Extra["codex_7d_reset_at"] = now.Add(-time.Second).Format(time.RFC3339)
	_, reason = accountSchedulingCandidateState(&account, now)
	require.Equal(t, "ready", reason)
	account.Schedulable = false
	_, reason = accountSchedulingCandidateState(&account, now)
	require.Equal(t, "administrator_disabled", reason)
	account.Credentials = map[string]any{"refresh_token": "not-a-stable-identity"}
	require.Empty(t, accountSchedulingIdentity(&account))
}

func TestAccountSchedulingUnknownTiersAndOrphanShadowsFailClosed(t *testing.T) {
	for _, plan := range []string{"", "free", "unknown", "abnormal", "unrecognized-new-tier"} {
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": plan}}
		require.True(t, accountSchedulingTarget(a), plan)
	}
	for _, plan := range []string{"plus", "pro", "team", "enterprise", "edu"} {
		a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": plan}}
		require.False(t, accountSchedulingTarget(a), plan)
	}
	parentID := int64(99)
	a := Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, ParentAccountID: &parentID, Credentials: map[string]any{"plan_type": "pro"}}
	candidates, err := BuildAccountSchedulingCandidates(AccountSchedulingPolicy{Enabled: true, MaxActiveAccounts: 3}, []Account{a}, nil, nil, nil, a.ID, "", time.Now())
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "identity_parent_missing", candidates[0].Reason)
	_, allowed := ReconcileAccountSchedulingSlots(candidates, nil, nil, 3, time.Now())
	value, governed := allowed[a.ID]
	require.True(t, governed)
	require.False(t, value)
}

// Embedded ports keep this fake narrow while allowing WS tests to drive the
// same service and lease cancellation path as production.
type accountSchedulingTestRepository struct {
	AccountSchedulingPoolRepository
	mu        sync.Mutex
	Decision  *AccountSchedulingDecision
	Requests  []AccountSchedulingAdmission
	Released  []string
	RefreshOK bool
	Err       error
}

func (r *accountSchedulingTestRepository) Reconcile(_ context.Context, request AccountSchedulingAdmission) (*AccountSchedulingDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Requests = append(r.Requests, request)
	return r.Decision, r.Err
}
func (r *accountSchedulingTestRepository) ReleaseLease(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Released = append(r.Released, id)
	return nil
}
func (r *accountSchedulingTestRepository) RefreshLease(context.Context, string, time.Duration) (bool, error) {
	return r.RefreshOK, r.Err
}

type accountSchedulingTestSettings struct {
	SettingRepository
	Raw string
	Err error
}

func (s *accountSchedulingTestSettings) GetValue(context.Context, string) (string, error) {
	return s.Raw, s.Err
}

func TestAccountSchedulingAdmissionFailuresAndLeaseProvider(t *testing.T) {
	repo := &accountSchedulingTestRepository{Decision: &AccountSchedulingDecision{Enabled: true, Allowed: map[int64]bool{1: true}, Acquired: true}, RefreshOK: true}
	settings := &accountSchedulingTestSettings{Raw: `{"enabled":true,"max_active_accounts":3}`}
	svc := NewAccountSchedulingPoolService(repo, nil, nil, settings)
	lease, err := svc.Acquire(context.Background(), 1, "")
	require.NoError(t, err)
	require.NotNil(t, lease)
	ctx := WithAccountSchedulingLeaseProvider(context.Background(), 1, "", func() *AccountSchedulingLease { return lease })
	reused, ok, err := AccountSchedulingProvidedLease(ctx, 1, "")
	require.NoError(t, err)
	require.True(t, ok)
	require.Same(t, lease, reused)
	_, _, err = AccountSchedulingProvidedLease(ctx, 1, "http://different.invalid")
	require.Error(t, err)
	lease.Release()
	lease.Release()
	require.Len(t, repo.Released, 1)
	_, _, err = AccountSchedulingProvidedLease(ctx, 1, "")
	require.ErrorIs(t, err, ErrAccountSchedulingLeaseLost)
	repo.Err = errors.New("database unavailable")
	_, err = svc.Acquire(context.Background(), 1, "")
	require.Error(t, err)
	settings.Err = errors.New("settings unavailable")
	_, err = svc.Acquire(context.Background(), 1, "")
	require.Error(t, err)
}

func TestAccountSchedulingLiveRejectsGovernedBeforeUpstreamAllocation(t *testing.T) {
	repo := &accountSchedulingTestRepository{Decision: &AccountSchedulingDecision{Enabled: true, Allowed: map[int64]bool{1: true}}}
	settings := &accountSchedulingTestSettings{Raw: `{"enabled":true,"max_active_accounts":3}`}
	svc := NewAccountSchedulingPoolService(repo, nil, nil, settings)
	gateway := &OpenAIGatewayService{accountSchedulingPools: svc}
	// No token or upstream dependencies are installed: rejecting here proves
	// the call cannot allocate upstream media before the policy check.
	_, err := gateway.createUpstreamLiveCall(context.Background(), &Account{ID: 1}, &LiveCallRequest{}, "")
	require.ErrorIs(t, err, ErrAccountSchedulingLiveUnsupported)
	require.Empty(t, repo.Requests[0].LeaseID)
	delete(repo.Decision.Allowed, 1)
	require.NoError(t, svc.ValidateLiveAccount(context.Background(), 1, ""), "known paid and API-key accounts remain supported")
	settings.Raw = `{"enabled":false,"max_active_accounts":3}`
	repo.Err = errors.New("should not query admission while disabled")
	require.NoError(t, svc.ValidateLiveAccount(context.Background(), 1, ""))
}
