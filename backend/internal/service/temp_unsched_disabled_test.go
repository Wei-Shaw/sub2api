package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type tempOptOutRepo struct {
	AccountRepository
	account                           *Account
	tempCalls, modelCalls, errorCalls int
}

func (r *tempOptOutRepo) GetByID(context.Context, int64) (*Account, error) { return r.account, nil }
func (r *tempOptOutRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.tempCalls++
	return nil
}
func (r *tempOptOutRepo) SetModelRateLimit(context.Context, int64, string, time.Time, ...string) error {
	r.modelCalls++
	return nil
}
func (r *tempOptOutRepo) SetError(context.Context, int64, string) error { r.errorCalls++; return nil }

type tempOptOutCache struct {
	TempUnschedCache
	reads int
}

func (c *tempOptOutCache) GetTempUnsched(context.Context, int64) (*TempUnschedState, error) {
	c.reads++
	return &TempUnschedState{UntilUnix: time.Now().Add(time.Hour).Unix()}, nil
}

func tempOptOutAccount() *Account {
	return &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"temp_unschedulable_disabled": true, "pool_mode": true}}
}

func TestTempUnschedDisabledScheduling(t *testing.T) {
	future := time.Now().Add(time.Hour)
	account := tempOptOutAccount()
	account.TempUnschedulableUntil = &future
	require.True(t, account.IsSchedulable())
	require.True(t, account.IsCredentialUsableForShadow())
	gateway := &OpenAIGatewayService{}
	gateway.BlockAccountScheduling(account, future, "transport_error")
	require.False(t, gateway.isOpenAIAccountRequestRuntimeBlocked(account, "test"))
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
	for _, value := range []any{false, nil, "true", 1} {
		account.Credentials["temp_unschedulable_disabled"] = value
		require.False(t, account.IsSchedulable())
	}
	account.Credentials["temp_unschedulable_disabled"] = true
	account.RateLimitResetAt = &future
	require.False(t, account.IsSchedulable(), "ordinary rate limits still apply")
	account.RateLimitResetAt = nil
	account.OverloadUntil = &future
	require.False(t, account.IsSchedulable(), "overload still applies")
	account.OverloadUntil = nil
	account.Schedulable = false
	require.False(t, account.IsSchedulable(), "manual pause still applies")
	account.Schedulable = true
	account.Status = StatusError
	require.False(t, account.IsSchedulable(), "permanent errors still apply")
}

func TestTempUnschedDisabledTriggerPaths(t *testing.T) {
	ctx := context.Background()
	account := tempOptOutAccount()
	repo := &tempOptOutRepo{account: account}
	settingsRepo := &openAIAPIKeyHealthSettingRepo{value: `{"enabled":true,"window_minutes":1,"failure_threshold":3,"cooldown_minutes":5}`}
	healthCache := &openAIAPIKeyHealthCacheStub{tripped: true}
	rl := NewRateLimitService(repo, nil, &config.Config{}, nil, healthCache)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))
	rl.SetOpenAIAPIKeyHealthCache(healthCache)
	rl.SetAccountRuntimeBlocker(&OpenAIGatewayService{})
	require.False(t, rl.ObserveOpenAIAPIKeyHealthFailure(ctx, account, &UpstreamFailoverError{StatusCode: 502}))
	require.Zero(t, healthCache.recordCalls)
	require.True(t, rl.handleOpenAI403(ctx, account, "forbidden", []byte(`{"error":"forbidden"}`)))
	require.False(t, rl.ApplyAccountSchedulingThreshold(ctx, account))
	require.False(t, rl.triggerStreamTimeoutTempUnsched(ctx, account, &StreamTimeoutSettings{TempUnschedMinutes: 5}, "test"))
	rule := TempUnschedulableRule{ErrorCode: 503, Keywords: []string{"unavailable"}, DurationMinutes: 5}
	require.True(t, rl.triggerTempUnschedulable(ctx, account, rule, 0, 503, "unavailable", []byte("unavailable"), "test"), "keep failover without account or model punishment")
	tempUnscheduleAccountForTransportError(ctx, repo, account, "connection refused")
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.tempUnscheduleOpenAITransportError(ctx, account, "connection refused")
	gateway.tempUnscheduleGrok(ctx, account, time.Minute, "test")
	provider := &AntigravityTokenProvider{accountRepo: repo}
	provider.markTempUnschedulable(account, errors.New("refresh failed"))
	rl.setCNProviderTempUnschedulable(ctx, account, time.Now().Add(time.Hour), "test", "test", "test")
	require.Zero(t, repo.tempCalls)
	require.Zero(t, repo.modelCalls)
	require.Zero(t, repo.errorCalls)
	require.Nil(t, account.TempUnschedulableUntil)
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
	// Disabling the override restores the existing behavior.
	account.Credentials["temp_unschedulable_disabled"] = false
	tempUnscheduleAccountForTransportError(ctx, repo, account, "connection refused")
	require.Equal(t, 1, repo.tempCalls)
}

func TestTempUnschedDisabledIgnoresStaleStatusCache(t *testing.T) {
	account := tempOptOutAccount()
	cache := &tempOptOutCache{}
	rl := NewRateLimitService(&tempOptOutRepo{account: account}, nil, &config.Config{}, nil, cache)
	state, err := rl.GetTempUnschedStatus(context.Background(), account.ID)
	require.NoError(t, err)
	require.Nil(t, state)
	require.Zero(t, cache.reads)
	// Turning the switch back off must not resurrect a cleared Redis cooldown.
	account.Credentials["temp_unschedulable_disabled"] = false
	state, err = rl.GetTempUnschedStatus(context.Background(), account.ID)
	require.NoError(t, err)
	require.Nil(t, state)
	require.Zero(t, cache.reads)
}

func TestTempUnschedDisabledKeepsAPIKeyAuthErrors(t *testing.T) {
	account := tempOptOutAccount()
	delete(account.Credentials, "pool_mode")
	repo := &tempOptOutRepo{account: account}
	rl := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	require.True(t, rl.HandleUpstreamError(context.Background(), account, http.StatusUnauthorized, nil, []byte(`{"error":{"message":"invalid api key"}}`)))
	require.Equal(t, 1, repo.errorCalls)
	require.Zero(t, repo.tempCalls)
}
