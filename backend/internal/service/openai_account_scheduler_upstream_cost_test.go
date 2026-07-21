package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type upstreamCostTrackingConcurrencyCache struct {
	ConcurrencyCache
	loadMap       map[int64]*AccountLoadInfo
	acquireLimits map[int64][]int
	releases      map[int64]int
	rejectAcquire bool
}

func (c *upstreamCostTrackingConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, maxConcurrency int, _ string) (bool, error) {
	if c.acquireLimits == nil {
		c.acquireLimits = make(map[int64][]int)
	}
	c.acquireLimits[accountID] = append(c.acquireLimits[accountID], maxConcurrency)
	return !c.rejectAcquire, nil
}

func (c *upstreamCostTrackingConcurrencyCache) ReleaseAccountSlot(_ context.Context, accountID int64, _ string) error {
	if c.releases == nil {
		c.releases = make(map[int64]int)
	}
	c.releases[accountID]++
	return nil
}

func (c *upstreamCostTrackingConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if load := c.loadMap[account.ID]; load != nil {
			copied := *load
			out[account.ID] = &copied
		}
	}
	return out, nil
}

func (c *upstreamCostTrackingConcurrencyCache) limits(accountID int64) []int {
	return append([]int(nil), c.acquireLimits[accountID]...)
}

func (c *upstreamCostTrackingConcurrencyCache) releaseCount(accountID int64) int {
	return c.releases[accountID]
}

func (c *upstreamCostTrackingConcurrencyCache) totalAcquires() int {
	total := 0
	for _, limits := range c.acquireLimits {
		total += len(limits)
	}
	return total
}

type upstreamCostCountingAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
	getCalls int
}

func (r *upstreamCostCountingAccountRepo) GetByID(_ context.Context, accountID int64) (*Account, error) {
	r.getCalls++
	account := r.accounts[accountID]
	if account == nil {
		return nil, errors.New("account not found")
	}
	cloned := *account
	return &cloned, nil
}

func (r *upstreamCostCountingAccountRepo) calls() int {
	return r.getCalls
}

func upstreamCostTestAccount(id int64, status string, rate float64, receivedAt time.Time, interval time.Duration) *Account {
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			UpstreamBillingProbeExtraKey: map[string]any{
				"status": status,
				"data": map[string]any{
					"billing_scope":             "token",
					"resolved_rate_multiplier":  rate,
					"peak_rate_enabled":         false,
					"effective_rate_multiplier": rate,
				},
				"received_at":     receivedAt.UTC().Format(time.RFC3339Nano),
				"fresh_until":     receivedAt.Add(2 * interval).UTC().Format(time.RFC3339Nano),
				"last_attempt_at": receivedAt.UTC().Format(time.RFC3339Nano),
				"next_probe_at":   receivedAt.Add(interval).UTC().Format(time.RFC3339Nano),
			},
		},
	}
}

func upstreamCostTestOAuthAccount(id int64) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
}

func TestAdvancedCostSchedulerUsesTopKOverflowWhenPreferredAccountIsKnownFull(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, expensive} {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
	}
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		cheap.ID:     {AccountID: cheap.ID, CurrentConcurrency: 1, LoadRate: 100},
		expensive.ID: {AccountID: expensive.ID},
	}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive}},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(cache),
	}
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, expensive.ID, selection.Account.ID)
	require.Empty(t, cache.limits(cheap.ID))
	require.Equal(t, []int{1}, cache.limits(expensive.ID))
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerCapsRejectedCostOverflowAcquires(t *testing.T) {
	selectionOrder := make([]openAIAccountCandidateScore, 0, 15_000)
	for id := int64(1); id <= 15_000; id++ {
		account := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account: account, loadInfo: &AccountLoadInfo{AccountID: id}, loadKnown: false,
		})
	}
	cache := &upstreamCostTrackingConcurrencyCache{rejectAcquire: true}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		concurrencyService: NewConcurrencyService(cache),
	}}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(
		context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, selectionOrder,
	)

	require.NoError(t, err)
	require.Nil(t, selection)
	require.Equal(t, openAIAccountSelectionProbeLimit, cache.totalAcquires())
}

func TestOpenAICostOverflowExpandedOnlyWhenCostAddsCandidates(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1, Extra: map[string]any{"openai_compact_supported": true}}},
		{account: &Account{ID: 2}},
	}
	plan := openAIAccountLoadPlan{candidates: candidates, topK: 1, includeOverflowFallback: true}
	require.True(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{}, plan))
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{RequireCompact: true}, plan),
		"one candidate per compact tier does not expand either tier's top-k")
	plan.topK = len(candidates)
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{}, plan))
	plan.includeOverflowFallback = false
	plan.topK = 1
	require.False(t, openAICostOverflowExpanded(OpenAIAccountScheduleRequest{}, plan))
}

func TestAdvancedSchedulerKnownFullOverflowStillFindsAvailableAccount(t *testing.T) {
	selectionOrder := make([]openAIAccountCandidateScore, 0, openAIAccountSelectionProbeLimit+2)
	for id := int64(1); id <= openAIAccountSelectionProbeLimit+1; id++ {
		account := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account:   account,
			loadInfo:  &AccountLoadInfo{AccountID: id, CurrentConcurrency: 1, LoadRate: 100},
			loadKnown: true,
		})
	}
	availableID := int64(openAIAccountSelectionProbeLimit + 2)
	available := &Account{ID: availableID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
		account: available, loadInfo: &AccountLoadInfo{AccountID: availableID}, loadKnown: true,
	})
	cache := &upstreamCostTrackingConcurrencyCache{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		concurrencyService: NewConcurrencyService(cache),
	}}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(
		context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, selectionOrder,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, availableID, selection.Account.ID)
	require.Equal(t, 1, cache.totalAcquires())
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerSharesProbeBudgetWithFallbackDBRechecks(t *testing.T) {
	const size = 15_000
	latestAccounts := make(map[int64]*Account, size)
	snapshotAccounts := make(map[int64]*Account, size)
	selectionOrder := make([]openAIAccountCandidateScore, 0, size)
	for id := int64(1); id <= size; id++ {
		stale := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
		latest := *stale
		latest.Status = StatusDisabled
		snapshotAccounts[id] = stale
		latestAccounts[id] = &latest
		selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
			account: stale, loadInfo: &AccountLoadInfo{AccountID: id}, loadKnown: false,
		})
	}
	repo := &upstreamCostCountingAccountRepo{accounts: latestAccounts}
	cache := &upstreamCostTrackingConcurrencyCache{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{accountsByID: snapshotAccounts}},
		concurrencyService: NewConcurrencyService(cache),
	}}
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrderWithBudget(context.Background(), req, selectionOrder, budget)
	require.NoError(t, err)
	require.Nil(t, selection)
	selection, _, _, _, err = scheduler.finishLoadBalanceSelectionFallback(
		context.Background(), req, openAIAccountLoadSelectionAttempt{selectionOrder: selectionOrder}, budget, openAISelectionFilterStats{},
	)

	require.Error(t, err)
	require.Nil(t, selection)
	require.Equal(t, openAIAccountSelectionProbeLimit, cache.totalAcquires())
	require.Equal(t, openAIAccountSelectionProbeLimit, repo.calls())
}

func TestAdvancedCostSchedulerKeepsCompactSupportedOverflowAheadOfUnknown(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	preferred := upstreamCostTestAccount(11, UpstreamBillingProbeStatusOK, 0.01, now.Add(-time.Minute), 30*time.Minute)
	overflow := upstreamCostTestAccount(12, UpstreamBillingProbeStatusOK, 0.1, now.Add(-time.Minute), 30*time.Minute)
	unknown := upstreamCostTestAccount(13, UpstreamBillingProbeStatusOK, 0.001, now.Add(-time.Minute), 30*time.Minute)
	preferred.Extra["openai_compact_supported"] = true
	overflow.Extra["openai_compact_supported"] = true
	for _, account := range []*Account{preferred, overflow, unknown} {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
	}
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		preferred.ID: {AccountID: preferred.ID, CurrentConcurrency: 1, LoadRate: 100},
		overflow.ID:  {AccountID: overflow.ID},
		unknown.ID:   {AccountID: unknown.ID},
	}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*preferred, *overflow, *unknown}},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(cache),
	}
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, true)
	require.NoError(t, err)
	require.Equal(t, overflow.ID, selection.Account.ID)
	require.Empty(t, cache.limits(preferred.ID))
	require.Equal(t, []int{1}, cache.limits(overflow.ID))
	require.Empty(t, cache.limits(unknown.ID))
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerUnknownLoadFailsOpen(t *testing.T) {
	account := &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	cache := &upstreamCostTrackingConcurrencyCache{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{concurrencyService: NewConcurrencyService(cache)}}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, []openAIAccountCandidateScore{{
		account: account, loadInfo: &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 99}, loadKnown: false,
	}})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, []int{1}, cache.limits(account.ID))
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerReleasesSlotWhenDBDisablesCandidate(t *testing.T) {
	stale := &Account{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	backup := &Account{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	disabled := *stale
	disabled.Status = StatusDisabled
	repo := &upstreamCostCountingAccountRepo{accounts: map[int64]*Account{stale.ID: &disabled, backup.ID: backup}}
	snapshot := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{stale.ID: stale, backup.ID: backup}}
	cache := &upstreamCostTrackingConcurrencyCache{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshot},
		concurrencyService: NewConcurrencyService(cache),
	}}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, []openAIAccountCandidateScore{
		{account: stale, loadInfo: &AccountLoadInfo{AccountID: stale.ID}, loadKnown: true},
		{account: backup, loadInfo: &AccountLoadInfo{AccountID: backup.ID}, loadKnown: true},
	})
	require.NoError(t, err)
	require.Equal(t, backup.ID, selection.Account.ID)
	require.Equal(t, 1, cache.releaseCount(stale.ID))
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerReacquiresOnceWhenDBConcurrencyChanges(t *testing.T) {
	stale := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 10}
	latest := *stale
	latest.Concurrency = 1
	repo := &upstreamCostCountingAccountRepo{accounts: map[int64]*Account{stale.ID: &latest}}
	snapshot := &openAISnapshotCacheStub{accountsByID: map[int64]*Account{stale.ID: stale}}
	cache := &upstreamCostTrackingConcurrencyCache{}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		accountRepo:        repo,
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshot},
		concurrencyService: NewConcurrencyService(cache),
	}}

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, []openAIAccountCandidateScore{{
		account: stale, loadInfo: &AccountLoadInfo{AccountID: stale.ID}, loadKnown: true,
	}})
	require.NoError(t, err)
	require.Equal(t, 1, selection.Account.Concurrency)
	require.Equal(t, []int{10, 1}, cache.limits(stale.ID))
	require.Equal(t, 1, cache.releaseCount(stale.ID))
	selection.ReleaseFunc()
}

func TestAdvancedSchedulerKnownFullPoolsDoNotRecheckDB(t *testing.T) {
	for _, size := range []int{100, 15_000} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			accounts := make(map[int64]*Account, size)
			selectionOrder := make([]openAIAccountCandidateScore, 0, size)
			for i := 1; i <= size; i++ {
				account := &Account{ID: int64(i), Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
				accounts[account.ID] = account
				selectionOrder = append(selectionOrder, openAIAccountCandidateScore{
					account:   account,
					loadInfo:  &AccountLoadInfo{AccountID: account.ID, CurrentConcurrency: 1, LoadRate: 100},
					loadKnown: true,
				})
			}
			repo := &upstreamCostCountingAccountRepo{accounts: accounts}
			cache := &upstreamCostTrackingConcurrencyCache{}
			scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
				accountRepo:        repo,
				schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{accountsByID: accounts}},
				concurrencyService: NewConcurrencyService(cache),
			}}

			selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, selectionOrder)
			require.NoError(t, err)
			require.Nil(t, selection)
			require.Zero(t, repo.calls())
			require.Zero(t, cache.totalAcquires())
		})
	}
}

func TestOpenAIFreshUpstreamBillingRateRecomputesPeakAtSelectionTime(t *testing.T) {
	receivedAt := time.Date(2026, 7, 13, 17, 30, 0, 0, time.UTC)
	account := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.4, receivedAt, time.Hour)
	snapshot, ok := account.Extra[UpstreamBillingProbeExtraKey].(map[string]any)
	require.True(t, ok)
	snapshot["data"] = map[string]any{
		"billing_scope":             "token",
		"resolved_rate_multiplier":  0.4,
		"peak_rate_enabled":         true,
		"peak_start":                "09:00",
		"peak_end":                  "18:00",
		"peak_rate_multiplier":      2.0,
		"applied_peak_multiplier":   2.0,
		"effective_rate_multiplier": 0.8,
		"timezone":                  "UTC",
	}

	duringPeak, ok := openAIFreshUpstreamBillingRate(account, time.Date(2026, 7, 13, 17, 59, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, 0.8, duringPeak)

	afterPeak, ok := openAIFreshUpstreamBillingRate(account, time.Date(2026, 7, 13, 18, 1, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, 0.4, afterPeak)
}

func TestOpenAIUpstreamCostFactorsSparseProbeIsNeutral(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := make([]*Account, 0, 10)
	accounts = append(accounts, upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 1, now.Add(-time.Minute), 30*time.Minute))
	for id := int64(2); id <= 10; id++ {
		accounts = append(accounts, &Account{
			ID:       id,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey: map[string]any{
					"status":          UpstreamBillingProbeStatusFailed,
					"last_attempt_at": now.UTC().Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).UTC().Format(time.RFC3339Nano),
				},
			},
		})
	}

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	for id := int64(1); id <= 10; id++ {
		require.Equal(t, openAIUpstreamCostNeutralFactor, factors[id])
	}
}

func TestOpenAIUpstreamCostFactorsCoverageShrinksSparseSignal(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
	}
	for id := int64(3); id <= 10; id++ {
		accounts = append(accounts, &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	}

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	center := math.Sqrt(0.03 * 0.8)
	require.InDelta(t, 0.5+0.2*(1/(1+0.03/center)-0.5), factors[1], 1e-12)
	require.InDelta(t, 0.5+0.2*(1/(1+0.8/center)-0.5), factors[2], 1e-12)
	require.Equal(t, openAIUpstreamCostNeutralFactor, factors[3])
}

func TestOpenAIUpstreamCostFactorsUseMedianAgainstOutlier(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.1, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.2, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 100, now.Add(-time.Minute), 30*time.Minute),
	}

	factors := openAIUpstreamCostFactors(accounts, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.InDelta(t, 2.0/3.0, factors[1], 1e-12)
	require.InDelta(t, 0.5, factors[2], 1e-12)
	require.InDelta(t, 1/(1+100/0.2), factors[3], 1e-12)
}

func TestOpenAILegacyUpstreamRateOrderRequiresComparableRates(t *testing.T) {
	now := time.Now()
	oneKnown := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.False(t, oneKnown.enabled)

	allEqual := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute),
	}, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.False(t, allEqual.enabled)

	distinct := newOpenAILegacyUpstreamRateOrder([]*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}, now, defaultOpenAIOAuthSchedulingRateMultiplier)
	require.True(t, distinct.enabled)
	require.Negative(t, distinct.compare(&Account{ID: 1}, &Account{ID: 2}))
	require.Negative(t, distinct.compare(&Account{ID: 2}, &Account{ID: 3}))
}

func TestOpenAISchedulingRatePlacesOAuthAtConfiguredReference(t *testing.T) {
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	oauth := upstreamCostTestOAuthAccount(2)
	expensive := upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.12, now.Add(-time.Minute), 30*time.Minute)

	order := newOpenAILegacyUpstreamRateOrder([]*Account{cheap, oauth, expensive}, now, 0.05)
	require.True(t, order.enabled)
	require.Negative(t, order.compare(cheap, oauth))
	require.Negative(t, order.compare(oauth, expensive))

	factors := openAIUpstreamCostFactors([]*Account{cheap, oauth, expensive}, now, 0.05)
	require.Greater(t, factors[cheap.ID], factors[oauth.ID])
	require.Greater(t, factors[oauth.ID], factors[expensive.ID])
}

func TestOpenAIGatewayServiceLegacyLowRatePriorityUsesConfiguredOAuthReference(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	oauth := upstreamCostTestOAuthAccount(2)
	expensive := upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.12, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, oauth, expensive} {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
	}
	cheap.Priority, oauth.Priority, expensive.Priority = 20, 10, 0

	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
		SettingKeyOpenAIOAuthSchedulingRateMultiplier:  "0.05",
	}}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *oauth, *expensive}},
		cache:            &schedulerTestGatewayCache{},
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)},
	}
	groupID := int64(1)

	first, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, cheap.ID, first.Account.ID)

	second, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", map[int64]struct{}{cheap.ID: {}}, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, oauth.ID, second.Account.ID)
}

func TestOpenAIModelsSelectionIgnoresTokenCostSignal(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(51, UpstreamBillingProbeStatusOK, 0.02, now.Add(-time.Minute), 30*time.Minute)
	expensive := upstreamCostTestAccount(52, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	for _, account := range []*Account{cheap, expensive} {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 1
	}
	cheap.Priority = 10
	expensive.Priority = 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
	}}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive}},
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)},
	}

	account, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, expensive.ID, account.ID)
}

func TestOpenAIGatewayServiceLegacyLowRatePriorityIsIndependentFromAdvancedScheduler(t *testing.T) {
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	accounts := []Account{*cheap, *expensive}
	groupID := int64(1)

	tests := []struct {
		name      string
		enabled   bool
		loadBatch bool
		loadErr   error
		wantID    int64
	}{
		{name: "switch off keeps priority first", loadBatch: true, wantID: 2},
		{name: "load batch", enabled: true, loadBatch: true, wantID: 1},
		{name: "load batch disabled", enabled: true, wantID: 1},
		{name: "load lookup failure", enabled: true, loadBatch: true, loadErr: errors.New("load unavailable"), wantID: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
				openAIAdvancedSchedulerSettingKey:              "false",
				SettingKeyOpenAILowUpstreamRatePriorityEnabled: strconv.FormatBool(tt.enabled),
			}}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = tt.loadBatch
			svc := &OpenAIGatewayService{
				accountRepo:      schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:            &schedulerTestGatewayCache{},
				cfg:              cfg,
				rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					loadBatchErr: tt.loadErr,
					loadMap: map[int64]*AccountLoadInfo{
						1: {AccountID: 1, LoadRate: 90},
						2: {AccountID: 2, LoadRate: 10},
					},
				}),
			}

			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, tt.wantID, selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestOpenAIGatewayServiceAdvancedSchedulerIgnoresLegacyLowRateSwitch(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "true",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
	}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(settings, cfg)},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIGatewayServiceLegacyLowRatePrioritySkipsCooledDownAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute)
	cheap.Status, cheap.Schedulable, cheap.Concurrency, cheap.Priority = StatusActive, true, 1, 10
	cooldownUntil := now.Add(time.Minute)
	cheap.TempUnschedulableUntil = &cooldownUntil
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	expensive.Status, expensive.Schedulable, expensive.Concurrency, expensive.Priority = StatusActive, true, 1, 0
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:              "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled: "true",
	}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive}},
		cache:            &schedulerTestGatewayCache{},
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1},
			2: {AccountID: 2},
		}}),
	}
	groupID := int64(1)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIFreshUpstreamBillingRateUsesFreshCachedSuccessOnly(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		account *Account
		wantOK  bool
	}{
		{name: "fresh", account: upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute), wantOK: true},
		{name: "zero rate", account: upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0, now.Add(-time.Minute), 30*time.Minute), wantOK: true},
		{name: "transient failure with fresh cache", account: upstreamCostTestAccount(3, UpstreamBillingProbeStatusFailed, 0.3, now.Add(-time.Minute), 30*time.Minute), wantOK: true},
		{name: "stale", account: upstreamCostTestAccount(4, UpstreamBillingProbeStatusOK, 0.3, now.Add(-61*time.Minute), 30*time.Minute)},
		{name: "future", account: upstreamCostTestAccount(5, UpstreamBillingProbeStatusOK, 0.3, now.Add(time.Minute), 30*time.Minute)},
		{name: "unsupported", account: upstreamCostTestAccount(6, UpstreamBillingProbeStatusUnsupported, 0.3, now.Add(-time.Minute), 30*time.Minute)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := openAIFreshUpstreamBillingRate(tt.account, now)
			require.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestBuildOpenAISelectionOrderIncludesOverflowOnlyForCostScheduling(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{}
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}, score: 3},
		{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}, score: 2},
		{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}, score: 1},
	}

	legacy := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{}, openAIAccountLoadPlan{
		candidates: candidates,
		topK:       1,
	})
	require.Len(t, legacy, 1)

	costAware := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{}, openAIAccountLoadPlan{
		candidates:              candidates,
		topK:                    1,
		includeOverflowFallback: true,
	})
	require.Equal(t, []int64{1, 2, 3}, []int64{
		costAware[0].account.ID,
		costAware[1].account.ID,
		costAware[2].account.ID,
	})
}

func TestBuildOpenAIAccountLoadPlanUsesCostOnlyForTokenScope(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestOAuthAccount(2),
		upstreamCostTestAccount(3, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.UpstreamCost = 1.5
	settings := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIOAuthSchedulingRateMultiplier: "0.05",
	}}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{
		cfg:              cfg,
		rateLimitService: &RateLimitService{settingService: NewSettingService(settings, cfg)},
	}}
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1},
		2: {AccountID: 2},
		3: {AccountID: 3},
	}

	tokenPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{UseUpstreamTokenCost: true}, accounts, loadMap)
	require.Greater(t, tokenPlan.candidates[0].score, tokenPlan.candidates[1].score)
	require.Greater(t, tokenPlan.candidates[1].score, tokenPlan.candidates[2].score)
	require.True(t, tokenPlan.includeOverflowFallback)

	otherPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, accounts, loadMap)
	require.Equal(t, otherPlan.candidates[0].score, otherPlan.candidates[1].score)
	require.Equal(t, otherPlan.candidates[1].score, otherPlan.candidates[2].score)
	require.False(t, otherPlan.includeOverflowFallback)
}

func TestBuildOpenAIAccountSchedulerScoreSnapshotUpstreamCostIsExactNoOpWithoutSignal(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 20},
		2: {AccountID: 2, LoadRate: 80},
	}
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{Priority: 1, Load: 1, Queue: 0.7, ErrorRate: 0.8, TTFT: 0.5}
	baseline := buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)
	weights.UpstreamCost = 1.5
	withCost := buildOpenAIAccountSchedulerScoreSnapshot(accounts, loadMap, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)

	require.Equal(t, baseline, withCost)
}

func TestBuildOpenAIAccountSchedulerScoreSnapshotUsesUpstreamCostSignal(t *testing.T) {
	now := time.Now()
	accounts := []*Account{
		upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.03, now.Add(-time.Minute), 30*time.Minute),
		upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute),
	}
	weights := GatewayOpenAIWSSchedulerScoreWeightsView{UpstreamCost: 1.5}
	scores := buildOpenAIAccountSchedulerScoreSnapshot(accounts, nil, weights, false, defaultOpenAIOAuthSchedulingRateMultiplier)

	require.Greater(t, scores[1].BaseScore, scores[2].BaseScore)
}
