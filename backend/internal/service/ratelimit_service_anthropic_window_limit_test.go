//go:build unit

package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type anthropicWindowLimitRepo struct {
	mockAccountRepoForGemini
	rateLimitCalls          int
	tempUnschedCalls        int
	lastRateLimitReset      time.Time
	modelRateLimitCalls     int
	lastModelRateLimitScope string
	lastModelRateLimitReset time.Time
	sessionWindowCalls      int
	lastExtraUpdates        map[string]any
}

func (r *anthropicWindowLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitReset = resetAt
	return nil
}

func (r *anthropicWindowLimitRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *anthropicWindowLimitRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, _ ...string) error {
	r.modelRateLimitCalls++
	r.lastModelRateLimitScope = scope
	r.lastModelRateLimitReset = resetAt
	return nil
}

func (r *anthropicWindowLimitRepo) UpdateSessionWindow(_ context.Context, _ int64, _, _ *time.Time, _ string) error {
	r.sessionWindowCalls++
	return nil
}

func (r *anthropicWindowLimitRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.lastExtraUpdates = updates
	return nil
}

func TestHandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule(t *testing.T) {
	resetAt := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.02")
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(resetAt.Unix(), 10))

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:       42,
		Type:     AccountTypeOAuth,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusTooManyRequests),
					"keywords":         []any{"rate limit"},
					"duration_minutes": float64(10),
				},
			},
		},
	}

	svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		headers,
		[]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's rate limit. Please try again later."}}`),
	)

	require.Zero(t, repo.tempUnschedCalls, "official Anthropic window limits should not be shortened by local temp-unsched rules")
	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, resetAt, repo.lastRateLimitReset)
}

// fable429Headers 构造 7d_oi（Fable 专属 7d 窗口）触发 429 的完整响应头，
// 数值取自真实抓包（5h/7d 均 allowed，仅 7d_oi rejected）。
func fable429Headers(reset5h, resetOI time.Time) http.Header {
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset5h.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.41")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(resetOI.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-7d-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.56")
	headers.Set("anthropic-ratelimit-unified-7d_oi-reset", strconv.FormatInt(resetOI.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-7d_oi-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-7d_oi-surpassed-threshold", "1.0")
	headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", "1.0")
	headers.Set("anthropic-ratelimit-unified-fallback-percentage", "0.5")
	headers.Set("anthropic-ratelimit-unified-overage-disabled-reason", "org_level_disabled")
	headers.Set("anthropic-ratelimit-unified-overage-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-representative-claim", "seven_day_overage_included")
	headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(resetOI.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-status", "rejected")
	return headers
}

func TestHandleUpstreamError_Anthropic7dOiOnlyMarksModelRateLimit(t *testing.T) {
	now := time.Now()
	reset5h := now.Add(2 * time.Hour).Truncate(time.Second)
	resetOI := now.Add(80 * time.Hour).Truncate(time.Second)
	headers := fable429Headers(reset5h, resetOI)

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:       42,
		Type:     AccountTypeOAuth,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusTooManyRequests),
					"keywords":         []any{"rate limit"},
					"duration_minutes": float64(10),
				},
			},
		},
	}

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		headers,
		[]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's rate limit. Please try again later."}}`),
		"claude-fable-5",
	)

	require.False(t, shouldDisable)
	require.Zero(t, repo.rateLimitCalls, "7d_oi (Fable-only) window must not mark the whole account rate limited")
	require.Zero(t, repo.tempUnschedCalls, "7d_oi window must not trigger local temp-unsched rules")
	require.Zero(t, repo.sessionWindowCalls, "7d_oi window must not rewrite the 5h session window as rejected")
	require.Equal(t, 1, repo.modelRateLimitCalls)
	require.Equal(t, anthropicFableRateLimitKey, repo.lastModelRateLimitScope)
	require.Equal(t, resetOI, repo.lastModelRateLimitReset)

	// 429 响应头也要被动采样，避免 7d F 进度条在限流期内冻结在旧值
	require.NotNil(t, repo.lastExtraUpdates)
	require.Equal(t, 1.0, repo.lastExtraUpdates["passive_usage_7d_oi_utilization"])
	require.Equal(t, resetOI.Unix(), repo.lastExtraUpdates["passive_usage_7d_oi_reset"])
	require.Equal(t, 0.41, repo.lastExtraUpdates["session_window_utilization"])
}

func TestHandleUpstreamError_Anthropic5hWindowStillWinsOver7dOi(t *testing.T) {
	// 5h 窗口 rejected 时必须仍按账号级限流处理（用 5h reset），同时记录 Fable 模型限流。
	now := time.Now()
	reset5h := now.Add(2 * time.Hour).Truncate(time.Second)
	resetOI := now.Add(80 * time.Hour).Truncate(time.Second)
	headers := fable429Headers(reset5h, resetOI)
	headers.Set("anthropic-ratelimit-unified-5h-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.0")

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, nil, "claude-fable-5")

	require.Equal(t, 1, repo.rateLimitCalls, "exhausted 5h window must still rate limit the account")
	require.Equal(t, reset5h, repo.lastRateLimitReset)
	require.Equal(t, 1, repo.modelRateLimitCalls)
	require.Equal(t, anthropicFableRateLimitKey, repo.lastModelRateLimitScope)
}

func TestHandleUpstreamError_AnthropicAccountWindowStillWinsOver7dOi(t *testing.T) {
	// 7d 窗口真超限时必须仍按账号级限流处理，同时记录 Fable 模型限流。
	now := time.Now()
	reset5h := now.Add(2 * time.Hour).Truncate(time.Second)
	resetOI := now.Add(80 * time.Hour).Truncate(time.Second)
	headers := fable429Headers(reset5h, resetOI)
	headers.Set("anthropic-ratelimit-unified-7d-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.02")

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, nil, "claude-fable-5")

	require.Equal(t, 1, repo.rateLimitCalls, "exhausted 7d window must still rate limit the account")
	require.Equal(t, resetOI, repo.lastRateLimitReset)
	require.Equal(t, 1, repo.modelRateLimitCalls, "Fable model rate limit should also be recorded")
	require.Equal(t, anthropicFableRateLimitKey, repo.lastModelRateLimitScope)
}

func TestHandleUpstreamError_Anthropic429Without7dOiKeepsLegacyBehavior(t *testing.T) {
	// 无 7d_oi 头、5h/7d 均未超限的 429：保持旧行为（按较早 reset 标记账号限流）。
	now := time.Now()
	reset5h := now.Add(2 * time.Hour).Truncate(time.Second)
	reset7d := now.Add(80 * time.Hour).Truncate(time.Second)

	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset5h.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.41")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset7d.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-7d-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.56")

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformAnthropic}

	svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, nil, "claude-fable-5")

	require.Zero(t, repo.modelRateLimitCalls, "no 7d_oi signal → no model rate limit")
	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, reset5h, repo.lastRateLimitReset, "legacy path picks the sooner reset")
	require.Equal(t, 1, repo.sessionWindowCalls)
}
