package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIRateLimitReconcileRepo struct {
	AccountRepository
	mu         sync.Mutex
	accounts   map[int64]*Account
	listErr    error
	clearErr   error
	clearCalls int
}

func (r *openAIRateLimitReconcileRepo) ListOpenAIRateLimitRecoveryCandidates(_ context.Context, _ time.Time, limit int) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]Account, 0, min(limit, len(r.accounts)))
	for id := int64(1); len(result) < limit; id++ {
		account, ok := r.accounts[id]
		if !ok {
			if id > 10000 {
				break
			}
			continue
		}
		result = append(result, cloneOpenAIRateLimitReconcileAccount(account))
	}
	return result, nil
}

func (r *openAIRateLimitReconcileRepo) ClearOpenAIRateLimitIfObserved(
	_ context.Context,
	accountID int64,
	observedRateLimitedAt time.Time,
	observedResetAt time.Time,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearCalls++
	if r.clearErr != nil {
		return false, r.clearErr
	}
	account := r.accounts[accountID]
	if account == nil || account.RateLimitedAt == nil || account.RateLimitResetAt == nil ||
		!account.RateLimitedAt.Equal(observedRateLimitedAt) ||
		!account.RateLimitResetAt.Equal(observedResetAt) {
		return false, nil
	}
	account.RateLimitedAt = nil
	account.RateLimitResetAt = nil
	return true, nil
}

func cloneOpenAIRateLimitReconcileAccount(account *Account) Account {
	copy := *account
	if account.Extra != nil {
		copy.Extra = make(map[string]any, len(account.Extra))
		for key, value := range account.Extra {
			copy.Extra[key] = value
		}
	}
	return copy
}

type openAIRateLimitQuotaQuerierStub struct {
	mu        sync.Mutex
	usage     *OpenAIQuotaUsage
	err       error
	onQuery   func(int64)
	queried   []int64
	started   chan struct{}
	continueC chan struct{}
}

func (q *openAIRateLimitQuotaQuerierStub) QueryUsageForReconciliation(_ context.Context, accountID int64) (*OpenAIQuotaUsage, error) {
	q.mu.Lock()
	q.queried = append(q.queried, accountID)
	started := q.started
	continueC := q.continueC
	onQuery := q.onQuery
	q.mu.Unlock()
	if onQuery != nil {
		onQuery(accountID)
	}
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if continueC != nil {
		<-continueC
	}
	return q.usage, q.err
}

func (q *openAIRateLimitQuotaQuerierStub) queriedIDs() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]int64(nil), q.queried...)
}

type openAIRateLimitRuntimeBlockerStub struct {
	mu         sync.Mutex
	generation map[int64]uint64
	clearCalls int
}

func (b *openAIRateLimitRuntimeBlockerStub) AccountSchedulingBlockGeneration(accountID int64) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.generation[accountID]
}

func (b *openAIRateLimitRuntimeBlockerStub) ClearAccountSchedulingBlockIfGeneration(accountID int64, observedGeneration uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.generation[accountID] != observedGeneration {
		return false
	}
	b.clearCalls++
	b.generation[accountID]++
	return true
}

func (b *openAIRateLimitRuntimeBlockerStub) installNewBlock(accountID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.generation[accountID]++
}

type openAIRateLimitLeaderLockStub struct {
	mu    sync.Mutex
	owner map[string]string
}

func (c *openAIRateLimitLeaderLockStub) TryAcquireLeaderLock(_ context.Context, key, owner string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner == nil {
		c.owner = make(map[string]string)
	}
	if _, exists := c.owner[key]; exists {
		return false, nil
	}
	c.owner[key] = owner
	return true, nil
}

func (c *openAIRateLimitLeaderLockStub) ReleaseLeaderLock(_ context.Context, key, owner string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner[key] == owner {
		delete(c.owner, key)
	}
	return nil
}

func availableOpenAIQuotaUsage() *OpenAIQuotaUsage {
	return &OpenAIQuotaUsage{
		FetchedAt: time.Now().Unix(),
		RateLimit: &OpenAIRateLimit{
			Allowed:      true,
			LimitReached: false,
			PrimaryWindow: &OpenAIRateLimitWindow{
				UsedPercent:        20,
				LimitWindowSeconds: 18000,
			},
			SecondaryWindow: &OpenAIRateLimitWindow{
				UsedPercent:        40,
				LimitWindowSeconds: 604800,
			},
		},
	}
}

func rateLimitedOpenAIAccount(id int64, now time.Time) *Account {
	rateLimitedAt := now.Add(-time.Hour)
	resetAt := now.Add(4 * 24 * time.Hour)
	return &Account{
		ID:               id,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitedAt:    &rateLimitedAt,
		RateLimitResetAt: &resetAt,
	}
}

func enabledOpenAIRateLimitReconcileConfig(maxAccounts, concurrency int) *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{
		OpenAIRateLimitReconcile: config.GatewayOpenAIRateLimitReconcileConfig{
			Enabled:             true,
			IntervalSeconds:     300,
			MaxAccountsPerCycle: maxAccounts,
			Concurrency:         concurrency,
		},
	}}
}

func TestOpenAIQuotaUsageConfirmsAvailable(t *testing.T) {
	tests := []struct {
		name  string
		usage *OpenAIQuotaUsage
		want  bool
	}{
		{name: "fresh available quota", usage: availableOpenAIQuotaUsage(), want: true},
		{name: "missing rate limit", usage: &OpenAIQuotaUsage{}, want: false},
		{name: "allowed without windows is insufficient", usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{Allowed: true},
		}},
		{name: "upstream denies requests", usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{Allowed: false, PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 0}},
		}},
		{name: "limit reached flag wins", usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{Allowed: true, LimitReached: true, PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 0}},
		}},
		{name: "any exhausted window blocks recovery", usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{
				Allowed:         true,
				PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 20},
				SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100},
			},
		}},
		{name: "negative usage is insufficient", usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{Allowed: true, PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: -1}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIQuotaUsageConfirmsAvailable(tt.usage))
		})
	}
}

func TestOpenAIRateLimitReconcilerIsOptIn(t *testing.T) {
	now := time.Now()
	repo := &openAIRateLimitReconcileRepo{accounts: map[int64]*Account{1: rateLimitedOpenAIAccount(1, now)}}
	querier := &openAIRateLimitQuotaQuerierStub{usage: availableOpenAIQuotaUsage()}
	service := NewOpenAIRateLimitReconcilerService(repo, querier, nil, &config.Config{})

	require.NoError(t, service.RunOnce(context.Background()))
	require.Empty(t, querier.queriedIDs())
	require.NotNil(t, repo.accounts[1].RateLimitResetAt)
}

func TestOpenAIRateLimitReconcilerRecoversOnlyEligibleAccounts(t *testing.T) {
	now := time.Now()
	eligible := rateLimitedOpenAIAccount(1, now)
	healthy := rateLimitedOpenAIAccount(2, now)
	healthy.RateLimitedAt = nil
	healthy.RateLimitResetAt = nil
	apiKey := rateLimitedOpenAIAccount(3, now)
	apiKey.Type = AccountTypeAPIKey
	modelLimited := rateLimitedOpenAIAccount(4, now)
	modelLimited.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			"gpt-5.4": map[string]any{"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339)},
		},
	}
	expiredModelLimit := rateLimitedOpenAIAccount(5, now)
	expiredModelLimit.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			"gpt-5.4": map[string]any{"rate_limit_reset_at": now.Add(-time.Hour).Format(time.RFC3339)},
		},
	}
	malformedModelLimit := rateLimitedOpenAIAccount(6, now)
	malformedModelLimit.Extra = map[string]any{
		modelRateLimitsKey: map[string]any{
			"gpt-5.4": map[string]any{"rate_limit_reset_at": "not-a-timestamp"},
		},
	}
	repo := &openAIRateLimitReconcileRepo{accounts: map[int64]*Account{
		1: eligible,
		2: healthy,
		3: apiKey,
		4: modelLimited,
		5: expiredModelLimit,
		6: malformedModelLimit,
	}}
	querier := &openAIRateLimitQuotaQuerierStub{usage: availableOpenAIQuotaUsage()}
	blocker := &openAIRateLimitRuntimeBlockerStub{generation: map[int64]uint64{1: 7, 5: 9}}
	service := NewOpenAIRateLimitReconcilerService(repo, querier, blocker, enabledOpenAIRateLimitReconcileConfig(20, 2))

	require.NoError(t, service.RunOnce(context.Background()))
	require.ElementsMatch(t, []int64{1, 5}, querier.queriedIDs())
	require.Equal(t, 2, repo.clearCalls)
	require.Equal(t, 2, blocker.clearCalls)
	require.Nil(t, repo.accounts[1].RateLimitResetAt)
	require.NotNil(t, repo.accounts[4].RateLimitResetAt)
}

func TestOpenAIRateLimitReconcilerDoesNotClearExhaustedQuota(t *testing.T) {
	now := time.Now()
	account := rateLimitedOpenAIAccount(1, now)
	usage := availableOpenAIQuotaUsage()
	usage.RateLimit.PrimaryWindow.UsedPercent = 100
	usage.RateLimit.LimitReached = true
	repo := &openAIRateLimitReconcileRepo{accounts: map[int64]*Account{1: account}}
	blocker := &openAIRateLimitRuntimeBlockerStub{generation: map[int64]uint64{1: 3}}
	service := NewOpenAIRateLimitReconcilerService(
		repo,
		&openAIRateLimitQuotaQuerierStub{usage: usage},
		blocker,
		enabledOpenAIRateLimitReconcileConfig(20, 2),
	)

	require.NoError(t, service.RunOnce(context.Background()))
	require.Zero(t, repo.clearCalls)
	require.Zero(t, blocker.clearCalls)
	require.NotNil(t, repo.accounts[1].RateLimitResetAt)
}

func TestOpenAIRateLimitReconcilerConcurrent429WinsBothCASGuards(t *testing.T) {
	now := time.Now()
	account := rateLimitedOpenAIAccount(1, now)
	repo := &openAIRateLimitReconcileRepo{accounts: map[int64]*Account{1: account}}
	blocker := &openAIRateLimitRuntimeBlockerStub{generation: map[int64]uint64{1: 11}}
	querier := &openAIRateLimitQuotaQuerierStub{usage: availableOpenAIQuotaUsage()}
	querier.onQuery = func(accountID int64) {
		blocker.installNewBlock(accountID)
		repo.mu.Lock()
		newLimitedAt := time.Now()
		newResetAt := time.Now().Add(7 * 24 * time.Hour)
		repo.accounts[accountID].RateLimitedAt = &newLimitedAt
		repo.accounts[accountID].RateLimitResetAt = &newResetAt
		repo.mu.Unlock()
	}
	service := NewOpenAIRateLimitReconcilerService(repo, querier, blocker, enabledOpenAIRateLimitReconcileConfig(20, 2))

	require.NoError(t, service.RunOnce(context.Background()))
	require.Equal(t, 1, repo.clearCalls)
	require.Zero(t, blocker.clearCalls)
	require.NotNil(t, repo.accounts[1].RateLimitResetAt)
}

func TestOpenAIRateLimitReconcilerOnlyOneInstanceRunsPerCadence(t *testing.T) {
	now := time.Now()
	repo := &openAIRateLimitReconcileRepo{accounts: map[int64]*Account{1: rateLimitedOpenAIAccount(1, now)}}
	querier := &openAIRateLimitQuotaQuerierStub{
		usage:     availableOpenAIQuotaUsage(),
		started:   make(chan struct{}, 1),
		continueC: make(chan struct{}),
	}
	cache := &openAIRateLimitLeaderLockStub{}
	first := NewOpenAIRateLimitReconcilerService(repo, querier, nil, enabledOpenAIRateLimitReconcileConfig(20, 2))
	second := NewOpenAIRateLimitReconcilerService(repo, querier, nil, enabledOpenAIRateLimitReconcileConfig(20, 2))
	first.SetLeaderLock(cache, nil)
	second.SetLeaderLock(cache, nil)

	firstDone := make(chan error, 1)
	go func() { firstDone <- first.RunOnce(context.Background()) }()
	<-querier.started

	require.NoError(t, second.RunOnce(context.Background()))
	close(querier.continueC)
	require.NoError(t, <-firstDone)
	require.NoError(t, second.RunOnce(context.Background()))
	require.Equal(t, []int64{1}, querier.queriedIDs())
}

func TestOpenAIRateLimitReconcilerPropagatesCandidateListFailure(t *testing.T) {
	repo := &openAIRateLimitReconcileRepo{listErr: errors.New("database unavailable")}
	service := NewOpenAIRateLimitReconcilerService(
		repo,
		&openAIRateLimitQuotaQuerierStub{usage: availableOpenAIQuotaUsage()},
		nil,
		enabledOpenAIRateLimitReconcileConfig(20, 2),
	)
	require.ErrorContains(t, service.RunOnce(context.Background()), "database unavailable")
}
