package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokFreeQuotaUsageRepoStub struct {
	UsageLogRepository

	batchResult map[int64]*usagestats.AccountStats
	batchErr    error
	batchCalls  int
	batchIDs    []int64
	batchStart  time.Time

	singleResult map[int64]*usagestats.AccountStats
	singleErr    error
	singleCalls  int
}

type grokFreeQuotaAccountRepoStub struct {
	AccountRepository

	rateLimitedCalls     int
	lastRateLimitedID    int64
	lastRateLimitResetAt time.Time
	activationCalls      int
	active               bool
}

func (r *grokFreeQuotaAccountRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedCalls++
	r.lastRateLimitedID = id
	r.lastRateLimitResetAt = resetAt
	return nil
}

func (r *grokFreeQuotaAccountRepoStub) SetRateLimitedIfLater(ctx context.Context, id int64, resetAt time.Time) error {
	return r.SetRateLimited(ctx, id, resetAt)
}

func (r *grokFreeQuotaAccountRepoStub) SetRateLimitedIfInactive(ctx context.Context, id int64, resetAt time.Time) (bool, error) {
	r.activationCalls++
	if r.active {
		return false, nil
	}
	r.active = true
	return true, r.SetRateLimited(ctx, id, resetAt)
}

func (r *grokFreeQuotaUsageRepoStub) GetAccountWindowStatsBatch(_ context.Context, accountIDs []int64, startTime time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.batchCalls++
	r.batchIDs = append([]int64(nil), accountIDs...)
	r.batchStart = startTime
	if r.batchErr != nil {
		return nil, r.batchErr
	}
	return r.batchResult, nil
}

func (r *grokFreeQuotaUsageRepoStub) GetAccountWindowStats(_ context.Context, accountID int64, _ time.Time) (*usagestats.AccountStats, error) {
	r.singleCalls++
	if r.singleErr != nil {
		return nil, r.singleErr
	}
	if stats := r.singleResult[accountID]; stats != nil {
		return stats, nil
	}
	return &usagestats.AccountStats{}, nil
}

func schedulableGrokFreeQuotaTestAccount(id int64, priority int) Account {
	return Account{
		ID:          id,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
	}
}

func TestGrokOAuthUsesFreeRollingQuota(t *testing.T) {
	monthlyLimit := 10.0
	legacyFreeLimit := int64(2_000_000)

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", account: nil, want: false},
		{name: "api key", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}, want: false},
		{name: "unprobed oauth defaults to free", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, want: true},
		{name: "explicit free tier", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "free"}}, want: true},
		{name: "paid credential", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"subscription_tier": "supergrok"}}, want: false},
		{name: "paid billing plan", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: &xai.BillingSummary{Plan: "SuperGrok"}}}, want: false},
		{name: "paid monthly allowance", account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: map[string]any{grokBillingExtraKey: &xai.BillingSummary{MonthlyLimitCents: &monthlyLimit}}}, want: false},
		{
			name: "paid evidence wins over stale free snapshot",
			account: &Account{
				Platform:    PlatformGrok,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"subscription_tier": "supergrok"},
				Extra: map[string]any{grokQuotaSnapshotExtraKey: &xai.QuotaSnapshot{
					Tokens: &xai.QuotaWindow{Limit: &legacyFreeLimit},
				}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grokOAuthUsesFreeRollingQuota(tt.account))
		})
	}
}

func TestGrokFreeRollingQuotaPrefetch_BoundaryAndRecovery(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	under := schedulableGrokFreeQuotaTestAccount(1, 0)
	atLimit := schedulableGrokFreeQuotaTestAccount(2, 0)
	over := schedulableGrokFreeQuotaTestAccount(3, 0)
	paid := schedulableGrokFreeQuotaTestAccount(4, 0)
	paid.Credentials = map[string]any{"subscription_tier": "supergrok"}
	accounts := []Account{under, atLimit, over, paid}

	repo := &grokFreeQuotaUsageRepoStub{batchResult: map[int64]*usagestats.AccountStats{
		under.ID:   {Tokens: xai.GrokFreeRolling24hTokenLimit - 1},
		atLimit.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit},
		over.ID:    {Tokens: xai.GrokFreeRolling24hTokenLimit + 1},
		paid.ID:    {Tokens: xai.GrokFreeRolling24hTokenLimit * 2},
	}}
	ctx := withGrokFreeQuotaUsagePrefetch(context.Background(), repo, accounts, now)

	exhausted, tokens, known := grokFreeRollingQuotaExhausted(ctx, repo, &under, now)
	require.True(t, known)
	require.False(t, exhausted)
	require.Equal(t, xai.GrokFreeRolling24hTokenLimit-1, tokens)

	exhausted, tokens, known = grokFreeRollingQuotaExhausted(ctx, repo, &atLimit, now)
	require.True(t, known)
	require.True(t, exhausted)
	require.Equal(t, xai.GrokFreeRolling24hTokenLimit, tokens)

	exhausted, _, known = grokFreeRollingQuotaExhausted(ctx, repo, &over, now)
	require.True(t, known)
	require.True(t, exhausted)

	exhausted, _, known = grokFreeRollingQuotaExhausted(ctx, repo, &paid, now)
	require.False(t, known)
	require.False(t, exhausted)
	require.Equal(t, []int64{under.ID, atLimit.ID, over.ID}, repo.batchIDs)
	require.Equal(t, now.Add(-24*time.Hour), repo.batchStart)
	require.Zero(t, repo.singleCalls)

	// A fresh scheduling pass sees the rolling window after old usage falls out
	// and automatically admits the account again.
	repo.batchResult[atLimit.ID] = &usagestats.AccountStats{Tokens: xai.GrokFreeRolling24hTokenLimit - 1}
	recoveredCtx := withGrokFreeQuotaUsagePrefetch(context.Background(), repo, []Account{atLimit}, now.Add(time.Minute))
	exhausted, tokens, known = grokFreeRollingQuotaExhausted(recoveredCtx, repo, &atLimit, now.Add(time.Minute))
	require.True(t, known)
	require.False(t, exhausted)
	require.Equal(t, xai.GrokFreeRolling24hTokenLimit-1, tokens)
}

func TestGrokFreeRollingQuotaPrefetch_BatchFailureFailsOpen(t *testing.T) {
	account := schedulableGrokFreeQuotaTestAccount(11, 0)
	repo := &grokFreeQuotaUsageRepoStub{
		batchErr:    errors.New("database unavailable"),
		singleErr:   errors.New("must not fall back to N+1"),
		batchResult: map[int64]*usagestats.AccountStats{},
	}
	ctx := withGrokFreeQuotaUsagePrefetch(context.Background(), repo, []Account{account}, time.Now())

	exhausted, _, known := grokFreeRollingQuotaExhausted(ctx, repo, &account, time.Now())
	require.False(t, known)
	require.False(t, exhausted)
	require.Equal(t, 1, repo.batchCalls)
	require.Zero(t, repo.singleCalls)
}

func TestPersistGrokFreeLocalUsageRateLimit(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	require.Equal(t, 24*time.Hour, grokFreeLocalUsageCooldown)

	tests := []struct {
		name      string
		account   Account
		exhausted bool
		known     bool
		wantCalls int
	}{
		{
			name:      "exhausted free account",
			account:   schedulableGrokFreeQuotaTestAccount(12, 0),
			exhausted: true,
			known:     true,
			wantCalls: 1,
		},
		{
			name:      "usage below limit",
			account:   schedulableGrokFreeQuotaTestAccount(13, 0),
			exhausted: false,
			known:     true,
		},
		{
			name:      "usage unknown",
			account:   schedulableGrokFreeQuotaTestAccount(14, 0),
			exhausted: true,
			known:     false,
		},
		{
			name: "existing active rate limit",
			account: func() Account {
				account := schedulableGrokFreeQuotaTestAccount(15, 0)
				resetAt := now.Add(time.Hour)
				account.RateLimitResetAt = &resetAt
				return account
			}(),
			exhausted: true,
			known:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &grokFreeQuotaAccountRepoStub{}
			persistGrokFreeLocalUsageRateLimit(context.Background(), repo, &tt.account, now, tt.exhausted, tt.known)

			require.Equal(t, tt.wantCalls, repo.rateLimitedCalls)
			if tt.wantCalls > 0 {
				require.Equal(t, tt.account.ID, repo.lastRateLimitedID)
				require.Equal(t, now.Add(grokFreeLocalUsageCooldown), repo.lastRateLimitResetAt)
			}
		})
	}
}

func TestGrokFreeRollingQuotaServiceWrappersPersistLocalCooldown(t *testing.T) {
	account := schedulableGrokFreeQuotaTestAccount(16, 0)
	usageRepo := &grokFreeQuotaUsageRepoStub{singleResult: map[int64]*usagestats.AccountStats{
		account.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit},
	}}

	t.Run("openai gateway", func(t *testing.T) {
		accountRepo := &grokFreeQuotaAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: accountRepo, usageLogRepo: usageRepo}
		before := time.Now()

		exhausted, tokens, known := svc.grokFreeRollingQuotaExhausted(context.Background(), &account)

		require.True(t, known)
		require.True(t, exhausted)
		require.Equal(t, xai.GrokFreeRolling24hTokenLimit, tokens)
		require.Equal(t, 1, accountRepo.rateLimitedCalls)
		require.WithinDuration(t, before.Add(grokFreeLocalUsageCooldown), accountRepo.lastRateLimitResetAt, time.Second)
	})

	t.Run("gateway", func(t *testing.T) {
		accountRepo := &grokFreeQuotaAccountRepoStub{}
		svc := &GatewayService{accountRepo: accountRepo, usageLogRepo: usageRepo}
		before := time.Now()

		exhausted, tokens, known := svc.grokFreeRollingQuotaExhausted(context.Background(), &account)

		require.True(t, known)
		require.True(t, exhausted)
		require.Equal(t, xai.GrokFreeRolling24hTokenLimit, tokens)
		require.Equal(t, 1, accountRepo.rateLimitedCalls)
		require.WithinDuration(t, before.Add(grokFreeLocalUsageCooldown), accountRepo.lastRateLimitResetAt, time.Second)
	})

	t.Run("paid account remains exempt", func(t *testing.T) {
		paid := account
		paid.Credentials = map[string]any{"subscription_tier": "supergrok"}
		accountRepo := &grokFreeQuotaAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: accountRepo, usageLogRepo: usageRepo}

		exhausted, _, known := svc.grokFreeRollingQuotaExhausted(context.Background(), &paid)

		require.False(t, known)
		require.False(t, exhausted)
		require.Zero(t, accountRepo.rateLimitedCalls)
	})

	t.Run("stale snapshot cannot extend active cooldown", func(t *testing.T) {
		accountRepo := &grokFreeQuotaAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: accountRepo, usageLogRepo: usageRepo}

		firstBefore := time.Now()
		firstExhausted, _, firstKnown := svc.grokFreeRollingQuotaExhausted(context.Background(), &account)
		firstResetAt := accountRepo.lastRateLimitResetAt
		time.Sleep(time.Millisecond)
		secondExhausted, _, secondKnown := svc.grokFreeRollingQuotaExhausted(context.Background(), &account)

		require.True(t, firstKnown)
		require.True(t, firstExhausted)
		require.True(t, secondKnown)
		require.True(t, secondExhausted)
		require.Equal(t, 2, accountRepo.activationCalls)
		require.Equal(t, 1, accountRepo.rateLimitedCalls)
		require.WithinDuration(t, firstBefore.Add(grokFreeLocalUsageCooldown), firstResetAt, time.Second)
		require.Equal(t, firstResetAt, accountRepo.lastRateLimitResetAt)
	})
}

func TestOpenAIGatewayService_GrokFreeRollingQuota_LegacyAndStickySkipExhausted(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	exhaustedAccount := schedulableGrokFreeQuotaTestAccount(21, 0)
	healthyAccount := schedulableGrokFreeQuotaTestAccount(22, 1)
	accounts := []Account{exhaustedAccount, healthyAccount}
	repo := &grokFreeQuotaUsageRepoStub{
		batchResult: map[int64]*usagestats.AccountStats{
			exhaustedAccount.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit},
			healthyAccount.ID:   {Tokens: xai.GrokFreeRolling24hTokenLimit - 1},
		},
		singleResult: map[int64]*usagestats.AccountStats{
			exhaustedAccount.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit},
			healthyAccount.ID:   {Tokens: xai.GrokFreeRolling24hTokenLimit - 1},
		},
	}
	groupID := int64(12001)

	newService := func(cache *schedulerTestGatewayCache) *OpenAIGatewayService {
		cfg := &config.Config{}
		cfg.Gateway.Scheduling.LoadBatchEnabled = false
		return &OpenAIGatewayService{
			accountRepo: &grokFreeQuotaAccountRepoStub{
				AccountRepository: schedulerTestOpenAIAccountRepo{accounts: accounts},
			},
			usageLogRepo:       repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		}
	}

	t.Run("normal selection", func(t *testing.T) {
		selection, _, err := newService(&schedulerTestGatewayCache{}).SelectAccountWithSchedulerForCapability(
			context.Background(), &groupID, "", "", "grok-4.3", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
			false, false, false, PlatformGrok,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, healthyAccount.ID, selection.Account.ID)
	})

	t.Run("sticky cannot bypass", func(t *testing.T) {
		const sessionHash = "grok-quota-sticky"
		cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{sessionHash: exhaustedAccount.ID}}
		selection, _, err := newService(cache).SelectAccountWithSchedulerForCapability(
			context.Background(), &groupID, "", sessionHash, "grok-4.3", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
			false, false, false, PlatformGrok,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, healthyAccount.ID, selection.Account.ID)
	})
}

func TestDefaultOpenAIAccountScheduler_GrokFreeRollingQuotaSkipsExhausted(t *testing.T) {
	exhaustedAccount := schedulableGrokFreeQuotaTestAccount(31, 0)
	healthyAccount := schedulableGrokFreeQuotaTestAccount(32, 1)
	repo := &grokFreeQuotaUsageRepoStub{batchResult: map[int64]*usagestats.AccountStats{
		exhaustedAccount.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit + 1},
		healthyAccount.ID:   {Tokens: xai.GrokFreeRolling24hTokenLimit - 1},
	}}
	svc := &OpenAIGatewayService{
		accountRepo: &grokFreeQuotaAccountRepoStub{
			AccountRepository: schedulerTestOpenAIAccountRepo{accounts: []Account{exhaustedAccount, healthyAccount}},
		},
		usageLogRepo: repo,
		cfg:          &config.Config{},
	}
	scheduler := newDefaultOpenAIAccountScheduler(svc, nil)

	selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
		Platform:           PlatformGrok,
		RequestedModel:     "grok-4.3",
		RequiredTransport:  OpenAIUpstreamTransportAny,
		RequiredCapability: OpenAIEndpointCapabilityChatCompletions,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, healthyAccount.ID, selection.Account.ID)
	require.Equal(t, 1, repo.batchCalls)
}

func TestGatewayService_GrokFreeRollingQuotaGate(t *testing.T) {
	account := schedulableGrokFreeQuotaTestAccount(41, 0)
	repo := &grokFreeQuotaUsageRepoStub{singleResult: map[int64]*usagestats.AccountStats{
		account.ID: {Tokens: xai.GrokFreeRolling24hTokenLimit},
	}}
	svc := &GatewayService{usageLogRepo: repo}

	require.False(t, svc.isAccountSchedulableForQuota(context.Background(), &account))
	account.Credentials = map[string]any{"subscription_tier": "supergrok"}
	require.True(t, svc.isAccountSchedulableForQuota(context.Background(), &account))
}
