//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func TestKiroQuotaFetcherCanFetch(t *testing.T) {
	f := NewKiroQuotaFetcher()

	require.True(t, f.CanFetch(&Account{Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social", "access_token": "at",
	}}))

	// 无可用凭证时不查。
	require.False(t, f.CanFetch(&Account{Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social",
	}}))

	require.False(t, f.CanFetch(&Account{Platform: PlatformAnthropic}))
	require.False(t, f.CanFetch(nil))
}

func TestKiroQuotaFetcherMapsUsageInfo(t *testing.T) {
	reset := time.Now().Add(48 * time.Hour).Unix()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/getUsageLimits", r.URL.Path)
		require.Equal(t, "AGENTIC_REQUEST", r.URL.Query().Get("resourceType"))
		require.Equal(t, "arn:x", r.URL.Query().Get("profileArn"))
		require.Contains(t, r.Header.Get("User-Agent"), "KiroIDE-")

		_, _ = w.Write([]byte(`{
			"subscriptionInfo":{"subscriptionTitle":"KIRO PRO+"},
			"overageConfiguration":{"overageStatus":"ENABLED"},
			"usageBreakdownList":[{
				"resourceType":"AGENTIC_REQUEST",
				"currentUsage":600,"usageLimit":1000,
				"nextDateReset":` + itoa(reset) + `,
				"bonuses":[{"usageLimit":200,"status":"ACTIVE"}]
			}]
		}`))
	}))
	defer srv.Close()

	f := NewKiroQuotaFetcher()
	f.qHostFor = func(*Account) string { return srv.URL }

	account := &Account{ID: 1, Platform: PlatformKiro, Credentials: map[string]any{
		"auth_method": "social", "access_token": "at", "profile_arn": "arn:x",
	}}

	res, err := f.FetchQuota(context.Background(), account, "")
	require.NoError(t, err)
	require.NotNil(t, res.UsageInfo)
	require.NotNil(t, res.Raw, "原始响应要留档")

	ui := res.UsageInfo
	require.Equal(t, "KIRO PRO+", ui.KiroSubscriptionTitle)
	require.Equal(t, "ENABLED", ui.KiroOverageStatus)

	require.NotNil(t, ui.KiroCredits)
	require.EqualValues(t, 600, ui.KiroCredits.UsedRequests)
	require.EqualValues(t, 1200, ui.KiroCredits.LimitRequests, "必须含 ACTIVE 赠送额度")
	require.InDelta(t, 50.0, ui.KiroCredits.Utilization, 0.01)
	require.NotNil(t, ui.KiroCredits.ResetsAt)
}

func TestKiroQuotaFetcherUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"expired"}`))
	}))
	defer srv.Close()

	f := NewKiroQuotaFetcher()
	f.qHostFor = func(*Account) string { return srv.URL }

	_, err := f.FetchQuota(context.Background(), &Account{
		ID: 1, Platform: PlatformKiro,
		Credentials: map[string]any{"auth_method": "social", "access_token": "at"},
	}, "")
	require.Error(t, err)
}

// TestKiroCreditsCooldownUsesRealResetTime 覆盖比 Antigravity 更准的一点：
// 冷却到上游给出的真实重置时间，而不是固定 5 小时。
func TestKiroCreditsCooldownUsesRealResetTime(t *testing.T) {
	now := time.Now()
	reset := now.Add(30 * time.Hour)

	b := &kiro.UsageBreakdown{CurrentUsage: 1200, UsageLimit: 1000, NextDateReset: &reset}
	until, ok := kiroCreditsCooldownUntil(b, now)
	require.True(t, ok)
	require.WithinDuration(t, reset, until, time.Second)

	// 未耗尽 → 不冷却。
	b.CurrentUsage = 500
	_, ok = kiroCreditsCooldownUntil(b, now)
	require.False(t, ok)
}

func TestKiroCreditsCooldownFallsBackWhenResetMissingOrStale(t *testing.T) {
	now := time.Now()

	// 缺 nextDateReset。
	b := &kiro.UsageBreakdown{CurrentUsage: 1000, UsageLimit: 1000}
	until, ok := kiroCreditsCooldownUntil(b, now)
	require.True(t, ok)
	require.WithinDuration(t, now.Add(kiroCreditsFallbackCooldown), until, time.Second)

	// nextDateReset 已过期 —— 直接用会导致立刻解冻并反复打上游。
	past := now.Add(-time.Hour)
	b.NextDateReset = &past
	until, ok = kiroCreditsCooldownUntil(b, now)
	require.True(t, ok)
	require.WithinDuration(t, now.Add(kiroCreditsFallbackCooldown), until, time.Second)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
