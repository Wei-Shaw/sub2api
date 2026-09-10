package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIOAuthCapacitySettingRepo struct {
	SettingRepository
	value string
}

func (r *openAIOAuthCapacitySettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, nil
}

type openAIOAuthCapacityAccountRepo struct {
	AccountRepository
	setCalls int
	until    time.Time
	setErr   error
}

func (r *openAIOAuthCapacityAccountRepo) SetOverloaded(_ context.Context, _ int64, until time.Time) error {
	r.setCalls++
	r.until = until
	return r.setErr
}

func (*openAIOAuthCapacityAccountRepo) SetError(context.Context, int64, string) error { return nil }

func (*openAIOAuthCapacityAccountRepo) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}

func (*openAIOAuthCapacityAccountRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}

type openAIOAuthCapacityCacheStub struct {
	recordCalls  int
	prepareCalls int
	ackCalls     int
	nextSequence int64
	outcomes     map[int64]bool
	pending      int64
	marker       int64
	markerUntil  time.Time
}

func (c *openAIOAuthCapacityCacheStub) BeginOpenAIOAuthCapacityAttempt(context.Context, int64) (int64, error) {
	c.nextSequence++
	return c.nextSequence, nil
}

func (c *openAIOAuthCapacityCacheStub) RecordOpenAIOAuthCapacityOutcome(_ context.Context, _ int64, sequence int64, failed bool, _ int, threshold int, _ int) (int64, int64, bool, error) {
	c.recordCalls++
	if c.outcomes == nil {
		c.outcomes = make(map[int64]bool)
	}
	if _, exists := c.outcomes[sequence]; !exists {
		c.outcomes[sequence] = failed
	}
	var count int64
	for current := c.nextSequence; current > 0; current-- {
		outcome, exists := c.outcomes[current]
		if !exists || !outcome {
			break
		}
		count++
	}
	if c.pending > 0 {
		return count, c.pending, true, nil
	}
	if count >= int64(threshold) {
		c.pending = c.nextSequence
		return count, c.pending, true, nil
	}
	return count, 0, false, nil
}

func (c *openAIOAuthCapacityCacheStub) PrepareOpenAIOAuthCapacityCooldown(_ context.Context, _ int64, tripSequence int64, until time.Time) error {
	c.prepareCalls++
	c.marker = tripSequence
	c.markerUntil = until
	return nil
}

func (c *openAIOAuthCapacityCacheStub) AcknowledgeOpenAIOAuthCapacityCooldown(_ context.Context, _ int64, tripSequence int64) error {
	c.ackCalls++
	for sequence := range c.outcomes {
		if sequence <= tripSequence {
			delete(c.outcomes, sequence)
		}
	}
	if c.pending <= tripSequence {
		c.pending = 0
	}
	return nil
}

func (c *openAIOAuthCapacityCacheStub) GetOpenAIOAuthCapacityCooldown(context.Context, int64) (int64, time.Time, bool, error) {
	return c.marker, c.markerUntil, c.marker > 0 && time.Now().Before(c.markerUntil), nil
}

type openAIOAuthCapacityRuntimeBlocker struct {
	calls  int
	reason string
	until  time.Time
}

func (b *openAIOAuthCapacityRuntimeBlocker) BlockAccountScheduling(_ *Account, until time.Time, reason string) {
	b.calls++
	b.reason = reason
	b.until = until
}

func (*openAIOAuthCapacityRuntimeBlocker) ClearAccountSchedulingBlock(int64) {}

func newOpenAIOAuthCapacityRateLimitService(t *testing.T, enabled bool, threshold int) (*RateLimitService, *openAIOAuthCapacityCacheStub, *openAIOAuthCapacityAccountRepo, *openAIOAuthCapacityRuntimeBlocker) {
	t.Helper()
	encoded, err := json.Marshal(OverloadCooldownSettings{
		Enabled:                             true,
		CooldownMinutes:                     10,
		OpenAIOAuthCapacityEnabled:          enabled,
		OpenAIOAuthCapacityWindowMinutes:    2,
		OpenAIOAuthCapacityFailureThreshold: threshold,
		OpenAIOAuthCapacityCooldownMinutes:  5,
	})
	require.NoError(t, err)

	settings := NewSettingService(&openAIOAuthCapacitySettingRepo{value: string(encoded)}, &config.Config{})
	cache := &openAIOAuthCapacityCacheStub{}
	repo := &openAIOAuthCapacityAccountRepo{}
	blocker := &openAIOAuthCapacityRuntimeBlocker{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settings)
	svc.SetOpenAIOAuthCapacityFailureCache(cache)
	svc.SetAccountRuntimeBlocker(blocker)
	return svc, cache, repo, blocker
}

func openAIOAuthCapacityAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func TestOpenAIOAuthCapacityCooldownDefaultDisabled(t *testing.T) {
	svc, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, false, 1)
	account := openAIOAuthCapacityAccount(42)
	payload := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)

	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), account, http.StatusServiceUnavailable, payload, ""))
	require.Zero(t, cache.recordCalls)
	require.Zero(t, repo.setCalls)
	require.Nil(t, account.OverloadUntil)
}

func TestOpenAIOAuthCapacityCooldownAggregatesHTTPAndStreamFailures(t *testing.T) {
	rateLimits, cache, repo, blocker := newOpenAIOAuthCapacityRateLimitService(t, true, 2)
	gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := openAIOAuthCapacityAccount(42)
	httpPayload := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)
	streamPayload := []byte(`{"type":"response.failed","response":{"error":{"code":"slow_down","message":"slow down"}}}`)

	require.False(t, gateway.handleOpenAIAccountUpstreamError(
		context.Background(), account, http.StatusServiceUnavailable, nil, httpPayload, "gpt-5.6-sol",
	))
	require.Equal(t, 1, cache.recordCalls)
	require.Zero(t, repo.setCalls)
	require.True(t, account.IsSchedulable())
	account.OpenAIOAuthCapacityAttemptSequence = 0

	statusCode, shouldDisable := gateway.handleOpenAIStreamTerminalAccountSideEffects(nil, account, streamPayload, "slow down", nil, "gpt-5.6-sol")
	require.Equal(t, http.StatusServiceUnavailable, statusCode)
	require.False(t, shouldDisable)
	require.Equal(t, 2, cache.recordCalls)
	require.Equal(t, 1, repo.setCalls)
	require.Equal(t, 1, blocker.calls)
	require.Equal(t, openAIOAuthCapacityCooldownReason, blocker.reason)
	require.NotNil(t, account.OverloadUntil)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), repo.until, 2*time.Second)
	require.Zero(t, repo.until.Nanosecond()%int(time.Microsecond), "cooldown deadline must survive PostgreSQL precision unchanged")
	require.WithinDuration(t, repo.until, blocker.until, time.Second)
	require.False(t, account.IsSchedulable())

	expired := time.Now().Add(-time.Second)
	account.OverloadUntil = &expired
	require.True(t, account.IsSchedulable(), "account must recover automatically after cooldown expiry")
}

func TestOpenAIOAuthCapacityCooldownIgnoresOtherAccountsAndErrors(t *testing.T) {
	svc, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 1)
	capacityPayload := []byte(`{"error":{"code":"server_is_overloaded"}}`)
	apiKey := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	oauth := openAIOAuthCapacityAccount(2)

	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), apiKey, http.StatusServiceUnavailable, capacityPayload, ""))
	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), oauth, http.StatusServiceUnavailable, []byte(`{"error":{"code":"server_error"}}`), "upstream failed"))
	require.Zero(t, cache.recordCalls)
	require.Zero(t, repo.setCalls)
}

func TestOpenAIOAuthCapacityCooldownMakesSchedulerChooseHealthyAccount(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10101)
	until := time.Now().Add(5 * time.Minute).Truncate(time.Microsecond)
	databaseUntil := time.UnixMicro(until.UnixMicro())
	cooled := &Account{ID: 31001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{groupID}, OverloadUntil: &databaseUntil}
	healthy := &Account{ID: 31002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, GroupIDs: []int64{groupID}}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:oauth_capacity": cooled.ID}}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{cooled, healthy},
		accountsByID:     map[int64]*Account{cooled.ID: cooled, healthy.ID: healthy},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cooled, *healthy}},
		cache:              cache,
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		schedulerSnapshot:  &SchedulerSnapshotService{cache: snapshotCache},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "oauth_capacity", "gpt-5.6-sol", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, healthy.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	cache.sessionBindings["openai:oauth_capacity"] = cooled.ID
	cooled.OpenAIOAuthCapacityAttemptSequence = 1
	retryCtx := context.WithValue(WithOpenAIOAuthCapacityCooldownRetry(ctx, cooled), openAIOAuthCapacityRetryContextKey{}, openAIOAuthCapacityRetry{
		accountID: cooled.ID, attemptSequence: 1, tripSequence: 1, overloadUntil: until,
	})
	retrySelection, retryDecision, err := svc.SelectAccountWithScheduler(retryCtx, &groupID, "", "oauth_capacity", "gpt-5.6-sol", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, retrySelection)
	require.Equal(t, cooled.ID, retrySelection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, retryDecision.Layer)
	if retrySelection.ReleaseFunc != nil {
		retrySelection.ReleaseFunc()
	}
}

func TestOpenAIOAuthCapacityRetryContextDoesNotBypassOtherOrNewerBlocks(t *testing.T) {
	until := time.Now().Add(5 * time.Minute)
	account := openAIOAuthCapacityAccount(42)
	account.OverloadUntil = &until
	account.OpenAIOAuthCapacityAttemptSequence = 1
	ctx := context.WithValue(WithOpenAIOAuthCapacityCooldownRetry(context.Background(), account), openAIOAuthCapacityRetryContextKey{}, openAIOAuthCapacityRetry{
		accountID: account.ID, attemptSequence: 1, tripSequence: 1, overloadUntil: until,
	})

	require.False(t, account.IsSchedulable())
	require.True(t, isOpenAIAccountSchedulableForContext(ctx, account))

	rateLimitedUntil := time.Now().Add(time.Minute)
	account.RateLimitResetAt = &rateLimitedUntil
	require.False(t, isOpenAIAccountSchedulableForContext(ctx, account))
	account.RateLimitResetAt = nil

	gateway := &OpenAIGatewayService{}
	gateway.BlockAccountScheduling(account, until, openAIOAuthCapacityCooldownReason)
	require.True(t, gateway.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.6-sol"))
	require.False(t, gateway.isOpenAIAccountRequestRuntimeBlockedForContext(ctx, account, "gpt-5.6-sol"))

	gateway.BlockAccountScheduling(account, until, "unrelated_overload")
	require.True(t, gateway.isOpenAIAccountRequestRuntimeBlockedForContext(ctx, account, "gpt-5.6-sol"))
	gateway.ClearAccountSchedulingBlock(account.ID)
	newerRuntimeUntil := until.Add(time.Minute)
	gateway.BlockAccountScheduling(account, newerRuntimeUntil, openAIOAuthCapacityCooldownReason)
	require.True(t, gateway.isOpenAIAccountRequestRuntimeBlockedForContext(ctx, account, "gpt-5.6-sol"))
	newerUntil := until.Add(time.Minute)
	account.OverloadUntil = &newerUntil
	require.False(t, isOpenAIAccountSchedulableForContext(ctx, account))

	other := openAIOAuthCapacityAccount(43)
	other.OverloadUntil = &until
	require.False(t, isOpenAIAccountSchedulableForContext(ctx, other))
}

func TestOpenAIOAuthCapacityCooldownDoesNotShortenExistingOverload(t *testing.T) {
	svc, _, repo, blocker := newOpenAIOAuthCapacityRateLimitService(t, true, 1)
	later := time.Now().Add(30 * time.Minute)
	account := openAIOAuthCapacityAccount(42)
	account.OverloadUntil = &later
	payload := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)

	require.True(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), account, http.StatusServiceUnavailable, payload, ""))
	require.True(t, repo.until.Before(later))
	require.WithinDuration(t, later, *account.OverloadUntil, time.Second)
	require.Zero(t, blocker.calls)
}

func TestOpenAIOAuthCapacitySemantic529CountsOnce(t *testing.T) {
	rateLimits, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 2)
	gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := openAIOAuthCapacityAccount(42)
	payload := []byte(`{"type":"response.failed","response":{"error":{"status_code":529,"code":"server_is_overloaded","message":"overloaded"}}}`)

	status, disabled := gateway.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, "overloaded", nil, "gpt-5.6-sol")
	require.Equal(t, 529, status)
	require.False(t, disabled)
	require.Equal(t, 1, cache.recordCalls)
	require.Zero(t, repo.setCalls)
}

func TestOpenAIOAuthCapacityWSPairCountsOnce(t *testing.T) {
	rateLimits, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 2)
	gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := openAIOAuthCapacityAccount(42)
	errorEvent := []byte(`{"type":"error","error":{"status_code":529,"code":"server_is_overloaded","message":"overloaded"}}`)
	failedEvent := []byte(`{"type":"response.failed","response":{"error":{"status_code":529,"code":"server_is_overloaded","message":"overloaded"}}}`)

	applied := gateway.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5.6-sol", nil, errorEvent)
	if !applied {
		applied = gateway.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5.6-sol", nil, failedEvent)
	}

	require.True(t, applied)
	require.Equal(t, 1, cache.recordCalls)
	require.Zero(t, repo.setCalls)
}

func TestOpenAIOAuthCapacitySuccessBreaksFailureStreak(t *testing.T) {
	svc, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 2)
	account := openAIOAuthCapacityAccount(42)
	payload := []byte(`{"error":{"code":"server_is_overloaded"}}`)

	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), account, http.StatusServiceUnavailable, payload, ""))
	account.OpenAIOAuthCapacityAttemptSequence = 0
	svc.ObserveOpenAIOAuthCapacityNonFailure(context.Background(), account)
	account.OpenAIOAuthCapacityAttemptSequence = 0
	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), account, http.StatusServiceUnavailable, payload, ""))

	require.Equal(t, 3, cache.recordCalls)
	require.Zero(t, repo.setCalls)
}

func TestOpenAIOAuthCapacityPendingTransitionRetriesPersistence(t *testing.T) {
	svc, cache, repo, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 1)
	account := openAIOAuthCapacityAccount(42)
	payload := []byte(`{"error":{"code":"server_is_overloaded"}}`)
	repo.setErr = errors.New("database unavailable")

	require.False(t, svc.ObserveOpenAIOAuthCapacityFailure(context.Background(), account, http.StatusServiceUnavailable, payload, ""))
	require.Equal(t, 1, cache.prepareCalls)
	require.Zero(t, cache.ackCalls)
	require.Equal(t, 1, repo.setCalls)

	repo.setErr = nil
	account.OpenAIOAuthCapacityAttemptSequence = 0
	svc.ObserveOpenAIOAuthCapacityNonFailure(context.Background(), account)
	require.Equal(t, 2, cache.prepareCalls)
	require.Equal(t, 1, cache.ackCalls)
	require.Equal(t, 2, repo.setCalls)
	require.NotNil(t, account.OverloadUntil)
}

func TestOpenAIOAuthCapacitySplitThresholdPreservesInflightRetries(t *testing.T) {
	rateLimitsA, cache, _, _ := newOpenAIOAuthCapacityRateLimitService(t, true, 2)
	repoB := &openAIOAuthCapacityAccountRepo{}
	blockerB := &openAIOAuthCapacityRuntimeBlocker{}
	rateLimitsB := NewRateLimitService(repoB, nil, &config.Config{}, nil, nil)
	rateLimitsB.SetSettingService(rateLimitsA.settingService)
	rateLimitsB.SetOpenAIOAuthCapacityFailureCache(cache)
	rateLimitsB.SetAccountRuntimeBlocker(blockerB)
	gatewayA := &OpenAIGatewayService{rateLimitService: rateLimitsA}
	gatewayB := &OpenAIGatewayService{rateLimitService: rateLimitsB}
	accountA := openAIOAuthCapacityAccount(42)
	accountB := openAIOAuthCapacityAccount(42)
	accountA.OpenAIOAuthCapacityAttemptSequence = rateLimitsA.beginOpenAIOAuthCapacityAttempt(context.Background(), accountA)
	accountB.OpenAIOAuthCapacityAttemptSequence = rateLimitsB.beginOpenAIOAuthCapacityAttempt(context.Background(), accountB)
	payload := []byte(`{"error":{"code":"server_is_overloaded"}}`)

	require.False(t, rateLimitsA.ObserveOpenAIOAuthCapacityFailure(context.Background(), accountA, http.StatusServiceUnavailable, payload, ""))
	retryA := WithOpenAIOAuthCapacityCooldownRetry(context.Background(), accountA)
	require.True(t, rateLimitsB.ObserveOpenAIOAuthCapacityFailure(context.Background(), accountB, http.StatusServiceUnavailable, payload, ""))
	retryB := WithOpenAIOAuthCapacityCooldownRetry(context.Background(), accountB)

	accountA.OverloadUntil = &cache.markerUntil
	resolvedA := gatewayA.resolveOpenAIOAuthCapacityRetry(retryA)
	resolvedB := gatewayB.resolveOpenAIOAuthCapacityRetry(retryB)
	require.True(t, isOpenAIAccountSchedulableForContext(resolvedA, accountA))
	require.True(t, isOpenAIAccountSchedulableForContext(resolvedB, accountB))
	future := openAIOAuthCapacityAccount(42)
	future.OverloadUntil = &cache.markerUntil
	require.False(t, isOpenAIAccountSchedulableForContext(context.Background(), future))

	gatewayA.BlockAccountScheduling(accountA, cache.markerUntil, "unrelated_overload")
	require.True(t, gatewayA.isOpenAIAccountRequestRuntimeBlockedForContext(resolvedA, accountA, "gpt-5.6-sol"))
}
