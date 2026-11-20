package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpdateSessionWindow_WeeklyUsageWithoutFiveHourHeaders(t *testing.T) {
	for _, window := range []string{"7d", "7d_oi"} {
		t.Run(window, func(t *testing.T) {
			repo := &sessionWindowMockRepo{}
			svc := newRateLimitServiceForTest(repo)
			reset := time.Now().Add(72 * time.Hour).Truncate(time.Second)
			account := &Account{
				ID: 42, Platform: PlatformAnthropic, Type: AccountTypeSetupToken,
				RateLimitResetAt: &reset,
			}
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-"+window+"-utilization", "0.63")
			headers.Set("anthropic-ratelimit-unified-"+window+"-reset", strconv.FormatInt(reset.Unix(), 10))

			svc.UpdateSessionWindow(context.Background(), account, headers)

			require.Empty(t, repo.sessionWindowCalls, "weekly headers must not create a synthetic 5h window")
			require.Empty(t, repo.clearRateLimitIDs, "missing 5h status must not clear a weekly rate limit")
			require.Len(t, repo.updateExtraCalls, 1)
			call := repo.updateExtraCalls[0]
			require.Equal(t, account.ID, call.ID)
			require.Equal(t, 0.63, call.Updates["passive_usage_"+window+"_utilization"])
			require.Equal(t, reset.Unix(), call.Updates["passive_usage_"+window+"_reset"])
			require.NotContains(t, call.Updates, "session_window_utilization")
			sampledRaw, ok := call.Updates["passive_usage_sampled_at"].(string)
			require.True(t, ok)
			sampled, err := time.Parse(time.RFC3339, sampledRaw)
			require.NoError(t, err)
			require.WithinDuration(t, time.Now(), sampled, 2*time.Second)
		})
	}
}

func TestUpdateSessionWindow_NoQuotaHeadersDoesNotWriteUsage(t *testing.T) {
	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)
	svc.UpdateSessionWindow(context.Background(), &Account{ID: 42}, http.Header{})
	require.Empty(t, repo.updateExtraCalls)
	require.Empty(t, repo.sessionWindowCalls)
}

type weeklyUsageRateLimitRepo struct {
	sessionWindowMockRepo
	rateLimitCalls    int
	resetAt           time.Time
	modelLimitCalls   int
	modelLimitScope   string
	modelLimitReset   time.Time
	modelLimitReasons []string
}

func (r *weeklyUsageRateLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.resetAt = resetAt
	return nil
}

func (r *weeklyUsageRateLimitRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reasons ...string) error {
	r.modelLimitCalls++
	r.modelLimitScope = scope
	r.modelLimitReset = resetAt
	r.modelLimitReasons = reasons
	return nil
}

func TestHandleUpstreamError_SamplesWeeklyUsageWithoutFiveHourHeaders(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).Truncate(time.Second)
	for _, scenario := range []string{"shared_no_cooldown", "shared_equal_cooldown", "shared_later_cooldown", "fable_only"} {
		t.Run(scenario, func(t *testing.T) {
			repo := &weeklyUsageRateLimitRepo{}
			svc := newRateLimitServiceForTest(repo)
			account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
			weeklyUtilization, fableUtilization := 1.0, 0.75
			weeklyStatus, fableStatus := "rejected", "allowed"
			switch scenario {
			case "shared_equal_cooldown":
				account.RateLimitResetAt = &reset
			case "shared_later_cooldown":
				later := reset.Add(time.Hour)
				account.RateLimitResetAt = &later
			case "fable_only":
				weeklyUtilization, fableUtilization = 0.75, 1.0
				weeklyStatus, fableStatus = "allowed", "rejected"
			}
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-7d-status", weeklyStatus)
			headers.Set("anthropic-ratelimit-unified-7d-utilization", strconv.FormatFloat(weeklyUtilization, 'f', -1, 64))
			headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset.Unix(), 10))
			headers.Set("anthropic-ratelimit-unified-7d_oi-status", fableStatus)
			headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", strconv.FormatFloat(fableUtilization, 'f', -1, 64))
			headers.Set("anthropic-ratelimit-unified-7d_oi-reset", strconv.FormatInt(reset.Unix(), 10))

			shouldDisable := svc.HandleUpstreamError(context.Background(), account, http.StatusTooManyRequests, headers, nil)

			require.False(t, shouldDisable)
			if scenario == "shared_no_cooldown" {
				require.Equal(t, 1, repo.rateLimitCalls)
				require.Equal(t, reset, repo.resetAt)
			} else {
				require.Zero(t, repo.rateLimitCalls, "preserve an existing cooldown and keep Fable-only limits model-scoped")
			}
			if scenario == "fable_only" {
				require.Equal(t, 1, repo.modelLimitCalls)
				require.Equal(t, anthropicFableRateLimitKey, repo.modelLimitScope)
				require.Equal(t, reset, repo.modelLimitReset)
				require.Equal(t, []string{anthropicFableWindowReason}, repo.modelLimitReasons)
			} else {
				require.Zero(t, repo.modelLimitCalls)
			}
			require.Empty(t, repo.sessionWindowCalls, "weekly exhaustion must not invent a 5h window")
			require.Len(t, repo.updateExtraCalls, 1, "sample once even when a window handler returns early")
			updates := repo.updateExtraCalls[0].Updates
			require.Equal(t, weeklyUtilization, updates["passive_usage_7d_utilization"])
			require.Equal(t, reset.Unix(), updates["passive_usage_7d_reset"])
			require.Equal(t, fableUtilization, updates["passive_usage_7d_oi_utilization"])
			require.Equal(t, reset.Unix(), updates["passive_usage_7d_oi_reset"])
		})
	}
}
