package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type cohortRepo struct {
	AccountRepository
	accounts []Account
}

func (r *cohortRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return r.accounts, nil
}

type cohortQuota struct {
	openAIAutoResetQuota
	usages map[int64]*OpenAIQuotaUsage
	fault  bool
}

func (q *cohortQuota) QueryUsage(_ context.Context, id int64) (*OpenAIQuotaUsage, error) {
	if q.fault {
		return nil, errors.New("network")
	}
	return q.usages[id], nil
}
func exhaustedUsage(now time.Time) *OpenAIQuotaUsage {
	return &OpenAIQuotaUsage{FetchedAt: now.Unix(), RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 18000, ResetAfterSeconds: 3600}}}
}
func TestResetCohortProviderGroupModel(t *testing.T) {
	now := time.Now()
	g := int64(7)
	target := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, GroupIDs: []int64{g}}
	other := target
	other.ID = 2
	tests := []struct {
		name      string
		mutate    func(*Account)
		exhausted bool
		want      bool
	}{
		{"same cohort available", func(a *Account) {}, false, false},
		{"same cohort exhausted", func(a *Account) {}, true, true},
		{"other provider", func(a *Account) { a.Platform = PlatformAnthropic }, false, true},
		{"other group", func(a *Account) { a.GroupIDs = []int64{9} }, false, true},
		{"other model", func(a *Account) {
			a.Credentials = map[string]any{"model_mapping": map[string]any{"only-other-model": "other"}}
		}, false, true},
		{"busy slots not exhausted", func(a *Account) { a.Concurrency = 1 }, false, false},
		{"network backoff not exhausted", func(a *Account) { v := now.Add(time.Hour); a.TempUnschedulableUntil = &v }, false, false},
		{"API key unknown", func(a *Account) { a.Type = AccountTypeAPIKey }, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := other
			tt.mutate(&peer)
			usage := exhaustedUsage(now)
			if !tt.exhausted {
				usage.RateLimit.PrimaryWindow.UsedPercent = 10
			}
			s := &OpenAIQuotaAutoResetService{accountRepo: &cohortRepo{accounts: []Account{target, peer}}, quota: &cohortQuota{usages: map[int64]*OpenAIQuotaUsage{2: usage}}}
			s.scopes.Store(int64(1), openAIResetScope{request: OpenAIAccountScheduleRequest{GroupID: &g, Platform: PlatformOpenAI, RequestedModel: "gpt-5", RequiredTransport: OpenAIUpstreamTransportAny}, expires: now.Add(time.Minute)})
			require.Equal(t, tt.want, s.exhaustedCohort(context.Background(), &target, exhaustedUsage(now), now))
		})
	}
}
func TestResetNativeEvidenceFailsClosed(t *testing.T) {
	now := time.Now()
	for _, kind := range []string{"missing", "stale", "future", "expired", "zero", "unknown-window"} {
		t.Run(kind, func(t *testing.T) {
			u := exhaustedUsage(now)
			switch kind {
			case "missing":
				u.FetchedAt = 0
			case "stale":
				u.FetchedAt = now.Add(-24 * time.Hour).Unix()
			case "future":
				u.FetchedAt = now.Add(time.Hour).Unix()
			case "expired":
				u.RateLimit.PrimaryWindow.ResetAt = now.Add(-time.Second).Unix()
			case "zero":
				u.RateLimit.PrimaryWindow.UsedPercent = 0
			case "unknown-window":
				u.RateLimit.PrimaryWindow.LimitWindowSeconds = 60
			}
			require.False(t, openAIResetNativeExhausted(u, now))
		})
	}
}

type cohortLease struct {
	mu    sync.Mutex
	owner string
}

func (l *cohortLease) TryAcquireLeaderLock(_ context.Context, _, owner string, _ time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner != "" {
		return false, nil
	}
	l.owner = owner
	return true, nil
}
func (l *cohortLease) ReleaseLeaderLock(_ context.Context, _, owner string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == owner {
		l.owner = ""
	}
	return nil
}
func TestResetPolicyConcurrentDecisionLease(t *testing.T) {
	l := &cohortLease{}
	a := &OpenAIQuotaAutoResetService{leaderLock: l}
	b := &OpenAIQuotaAutoResetService{leaderLock: l}
	release, ok := a.acquireResetPolicyLease(context.Background())
	require.True(t, ok)
	_, ok = b.acquireResetPolicyLease(context.Background())
	require.False(t, ok)
	release()
	release, ok = b.acquireResetPolicyLease(context.Background())
	require.True(t, ok)
	release()
	_, ok = (&OpenAIQuotaAutoResetService{}).acquireResetPolicyLease(context.Background())
	require.False(t, ok)
}
func TestResetPolicyConfigModes(t *testing.T) {
	for _, mode := range []string{"threshold", "exhausted"} {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{OpenAIAutoResetCreditModeExtraKey: mode, OpenAIAutoResetCreditEnabledExtraKey: true})
		require.NoError(t, err)
		require.Equal(t, mode, ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}).Mode)
	}
	_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{OpenAIAutoResetCreditModeExtraKey: "unknown"})
	require.Error(t, err)
}

type policyWorkerRepo struct{ *autoResetTestAccountRepo }

func (r *policyWorkerRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []Account{*r.account}, nil
}
func TestResetPolicyWorkerConcurrentRedemption(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 99, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  100.0,
			"codex_7d_used_percent":                  10.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
			"codex_7d_reset_at":                      now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	account.Extra[OpenAIAutoResetCreditModeExtraKey] = OpenAIAutoResetModeExhausted
	repo := &policyWorkerRepo{autoResetTestAccountRepo: &autoResetTestAccountRepo{account: account}}
	lease := &cohortLease{}
	usage := &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 7 * 24 * 60 * 60, ResetAfterSeconds: 86400, ResetAt: now.Add(24 * time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
		},
		autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "credit-sensitive-id", ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
	}
	quota := &autoResetTestQuota{usage: usage, resetEntered: make(chan struct{}), releaseReset: make(chan struct{})}
	idempotencyRepo := newInMemoryIdempotencyRepo()
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	config.ProcessingTimeout = time.Second
	serviceA := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, lease)
	serviceB := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, lease)

	for _, svc := range []*OpenAIQuotaAutoResetService{serviceA, serviceB} {
		svc.scopes.Store(account.ID, openAIResetScope{request: OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5", RequiredTransport: OpenAIUpstreamTransportAny}, expires: now.Add(time.Minute)})
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = serviceA.evaluateAccount(context.Background(), account.ID)
	}()
	<-quota.resetEntered
	go func() {
		defer wg.Done()
		_ = serviceB.evaluateAccount(context.Background(), account.ID)
	}()
	time.Sleep(50 * time.Millisecond)
	close(quota.releaseReset)
	wg.Wait()

	require.Equal(t, int32(1), quota.resetCalls.Load())
	repo.mu.Lock()
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	repo.mu.Unlock()
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	encodedState, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(encodedState), "credit-sensitive-id")
}
