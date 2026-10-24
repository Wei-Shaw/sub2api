//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const zhipuFrequencyLimitBody = `{"error":{"code":"1302","message":"[1302][您的账户已达到速率限制，请您控制请求频率]","type":"rate_limit_error"}}`

// #6804: frequency-type 429s must use short backoff, never window cooldown.
func TestCnProviderResponseIsFrequencyLimit(t *testing.T) {
	require.True(t, cnProviderResponseIsFrequencyLimit([]byte(zhipuFrequencyLimitBody)))
	require.True(t, cnProviderResponseIsFrequencyLimit([]byte(`rate_limit_error: frequency exceeded`)))
	require.True(t, cnProviderResponseIsFrequencyLimit([]byte(`{"error":{"message":"FREQUENCY limit"}}`)))
	require.False(t, cnProviderResponseIsFrequencyLimit([]byte(`{"error":{"code":"insufficient_quota","message":"quota exhausted"}}`)))
	require.False(t, cnProviderResponseIsFrequencyLimit(nil))
}

func TestCnProviderQuotaNearlyExhausted(t *testing.T) {
	mkAccount := func(extra map[string]any) *Account {
		return &Account{Extra: extra}
	}
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": 97.0})))
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"codex_5h_used_percent": 95.0})))
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": json.Number("96.5")})))
	require.True(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_weekly_used_percent": "98"})))
	require.False(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": 7.0, "zhipu_weekly_used_percent": 1.0})))
	require.False(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{"zhipu_5h_used_percent": json.Number("7")})))
	require.False(t, cnProviderQuotaNearlyExhausted(mkAccount(map[string]any{})))
	require.False(t, cnProviderQuotaNearlyExhausted(nil))
}

func newZhipuCodingPlan429Account(used5h any, resetIn time.Duration) *Account {
	extra := map[string]any{}
	if resetIn > 0 {
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffix5hUsed)] = used5h
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffix5hReset)] = time.Now().Add(resetIn).UTC().Format(time.RFC3339)
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyUsed)] = 1.0
		extra[cnExtraKey(PlatformZhipu, cnExtraSuffixWeeklyReset)] = time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return &Account{
		ID:          7105,
		Platform:    PlatformZhipu,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       extra,
	}
}

func runZhipu429(svc *RateLimitService, account *Account, body string) {
	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, []byte(body))
}

func TestHandleUpstreamError_CNCodingPlanFrequency429UsesShortBackoff(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlan429Account(7.0, 2*time.Hour)
	before := time.Now()

	runZhipu429(svc, account, zhipuFrequencyLimitBody)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	shortUntil := before.Add(time.Duration(defaultRateLimit429CooldownSeconds) * time.Second)
	require.False(t, repo.lastRateLimitedAt.Before(shortUntil.Add(-2*time.Second)))
	require.False(t, repo.lastRateLimitedAt.After(time.Now().Add(time.Duration(defaultRateLimit429CooldownSeconds)*time.Second+2*time.Second)))
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(time.Minute)), "frequency 429 must not cool down to the window reset")
}

func TestHandleUpstreamError_CNCodingPlanLowQuota429UsesShortBackoff(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlan429Account(7.0, 2*time.Hour)

	runZhipu429(svc, account, `{"error":{"code":"rate_limit","message":"quota window busy"}}`)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(time.Minute)))
}

func TestHandleUpstreamError_CNCodingPlanNearlyExhausted429CoolsToWindowReset(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlan429Account(97.0, 2*time.Hour)

	runZhipu429(svc, account, `{"error":{"code":"rate_limit","message":"quota window busy"}}`)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.True(t, repo.lastRateLimitedAt.After(time.Now().Add(time.Hour)))
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(3*time.Hour)), "should cool down to the nearer 5h reset, not weekly")
}

func TestHandleUpstreamError_CNCodingPlanFrequency429StaysShortWhenQuotaNearlyExhausted(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newZhipuCodingPlan429Account(json.Number("99"), 2*time.Hour)

	runZhipu429(svc, account, zhipuFrequencyLimitBody)

	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(time.Minute)), "1302 is a frequency limit even when the snapshot is nearly full")
}
