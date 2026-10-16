//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const miniMax2062Body = `{"error":{"http_code":"429","message":"已达到 Token Plan 速率限制：请升级 Token Plan 套餐或切换为按量付费 API 使用。 (2062)","type":"rate_limit_error"},"type":"error"}`

func miniMaxQuotaAccount() *Account {
	return &Account{ID: 1, Platform: PlatformMiniMax, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra: map[string]any{"minimax_5h_used_percent": 25,
			"minimax_5h_reset_at":     time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339),
			"minimax_weekly_reset_at": time.Now().Add(6 * 24 * time.Hour).UTC().Format(time.RFC3339)}}
}

func TestMiniMax2062DoesNotUseQuotaWindow(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	blocker := &runtimeBlockRecorder{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetAccountRuntimeBlocker(blocker)
	before := time.Now()
	svc.HandleUpstreamError(context.Background(), miniMaxQuotaAccount(), http.StatusTooManyRequests, http.Header{}, []byte(miniMax2062Body))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.After(before.Add(59*time.Second)))
	require.True(t, repo.lastRateLimitedAt.Before(time.Now().Add(61*time.Second)), "2062 must not cool a healthy plan to its 5h reset")
	require.Zero(t, repo.setErrorCalls)
	require.Zero(t, repo.tempCalls)
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, "minimax_request_rate_limit", blocker.reasons[0])
}

func TestMiniMax2062RetryAfterAndExistingQuotaCooldown(t *testing.T) {
	for _, tc := range []struct {
		name     string
		retry    string
		existing time.Duration
		want     time.Duration
	}{
		{"retry_after", "90", 0, 90 * time.Second},
		{"preserve_quota_cooldown", "30", 2 * time.Hour, 2 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := miniMaxQuotaAccount()
			now := time.Now()
			if tc.existing > 0 {
				until := now.Add(tc.existing)
				account.RateLimitResetAt = &until
			}
			svc.HandleUpstreamError(context.Background(), account, 429, http.Header{"Retry-After": []string{tc.retry}}, []byte(miniMax2062Body))
			require.WithinDuration(t, now.Add(tc.want), repo.lastRateLimitedAt, time.Second)
		})
	}
}

func TestMiniMaxRequestRateResetUsesFullMinuteAcrossBoundary(t *testing.T) {
	now := time.Date(2026, 10, 13, 22, 22, 59, 0, time.UTC)
	require.Equal(t, now.Add(time.Minute), miniMaxRequestRateReset(now, http.Header{}))
	require.Equal(t, now.Add(time.Minute), miniMaxRequestRateReset(now, http.Header{"Retry-After": []string{"invalid"}}))
	until := now.Add(2 * time.Minute)
	require.Equal(t, until, miniMaxRequestRateReset(now, http.Header{"Retry-After": []string{until.Format(http.TimeFormat)}}))
}

func TestMiniMax2062Classification(t *testing.T) {
	account := miniMaxQuotaAccount()
	for _, body := range []string{miniMax2062Body, `{"base_resp":{"status_code":2062}}`, `{"error":{"code":"2062"}}`} {
		require.True(t, isMiniMaxRequestRateLimit(account, []byte(body)))
	}
	require.False(t, isMiniMaxRequestRateLimit(account, []byte(`{"error":{"code":1008,"message":"Quota exhausted"}}`)))
	require.False(t, isMiniMaxRequestRateLimit(&Account{Platform: PlatformOpenAI}, []byte(miniMax2062Body)))
}

func TestMiniMaxQuotaExhaustionKeepsWindowCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.HandleUpstreamError(context.Background(), miniMaxQuotaAccount(), 429, http.Header{}, []byte(`{"error":{"message":"Token Plan quota exhausted","code":1008}}`))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.True(t, repo.lastRateLimitedAt.After(time.Now().Add(2*time.Hour)))
}
