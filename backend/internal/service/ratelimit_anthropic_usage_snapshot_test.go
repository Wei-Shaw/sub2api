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

// Keep a persisted snapshot so the assertions use the same usage reader as the
// accounts page, rather than only checking the writes requested by the service.
type anthropicUsageSnapshotRepo struct {
	anthropicWindowLimitRepo
	account *Account
}

func (r *anthropicUsageSnapshotRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *anthropicUsageSnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

func TestUpdateSessionWindow_WeeklyUsageWithoutFiveHourStatus(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	account := &Account{
		ID: 11, Platform: PlatformAnthropic, Type: AccountTypeSetupToken,
		Extra: map[string]any{
			"passive_usage_7d_utilization":    0.5,
			"passive_usage_7d_reset":          reset.Unix(),
			"passive_usage_7d_oi_utilization": 0.25,
			"passive_usage_7d_oi_reset":       reset.Unix(),
		},
	}
	repo := &anthropicUsageSnapshotRepo{account: account}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.75")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-7d_oi-utilization", "0.9")
	headers.Set("anthropic-ratelimit-unified-7d_oi-reset", strconv.FormatInt(reset.Unix(), 10))

	svc.UpdateSessionWindow(t.Context(), account, headers)

	reader := &AccountUsageService{accountRepo: repo, cache: &UsageCache{}, usageLogRepo: &usageBatchLogRepoStub{}}
	usage, err := reader.GetPassiveUsage(t.Context(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage.SevenDay)
	require.Equal(t, 75.0, usage.SevenDay.Utilization)
	require.True(t, reset.Equal(*usage.SevenDay.ResetsAt))
	require.NotNil(t, usage.SevenDayFable)
	require.Equal(t, 90.0, usage.SevenDayFable.Utilization)
	require.NotNil(t, usage.UpdatedAt)
	require.Zero(t, repo.sessionWindowCalls, "weekly sampling must not invent a five-hour window")
	require.Zero(t, usage.FiveHour.Utilization)
}

func TestHandleUpstreamError_AnthropicWeeklyUsageRefreshesOn429(t *testing.T) {
	reset := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	for _, existingCooldown := range []string{"none", "same", "later"} {
		t.Run(existingCooldown, func(t *testing.T) {
			account := &Account{
				ID: 11, Platform: PlatformAnthropic, Type: AccountTypeSetupToken,
				Extra: map[string]any{
					"passive_usage_7d_utilization": 0.5,
					"passive_usage_7d_reset":       reset.Unix(),
				},
			}
			if existingCooldown != "none" {
				until := reset
				if existingCooldown == "later" {
					until = until.Add(time.Hour)
				}
				account.RateLimitResetAt = &until
			}
			repo := &anthropicUsageSnapshotRepo{account: account}
			svc := NewRateLimitService(repo, nil, nil, nil, nil)
			// Production Ultra response: no 5h headers, shared weekly quota
			// exhausted. fallback-percentage is not the utilization value.
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.0")
			headers.Set("anthropic-ratelimit-unified-7d-status", "rejected")
			headers.Set("anthropic-ratelimit-unified-7d-surpassed-threshold", "1.0")
			headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset.Unix(), 10))
			headers.Set("anthropic-ratelimit-unified-representative-claim", "seven_day")
			headers.Set("anthropic-ratelimit-unified-fallback-percentage", "0.5")
			headers.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(reset.Unix(), 10))

			shouldDisable := svc.HandleUpstreamError(t.Context(), account, http.StatusTooManyRequests, headers,
				[]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's rate limit. Please try again later."}}`), "claude-opus-5-5")

			require.False(t, shouldDisable)
			reader := &AccountUsageService{accountRepo: repo, cache: &UsageCache{}, usageLogRepo: &usageBatchLogRepoStub{}}
			usage, err := reader.GetPassiveUsage(t.Context(), account.ID)
			require.NoError(t, err)
			require.NotNil(t, usage.SevenDay)
			require.Equal(t, 100.0, usage.SevenDay.Utilization)
			require.True(t, reset.Equal(*usage.SevenDay.ResetsAt))
			require.NotNil(t, usage.UpdatedAt)
			require.Zero(t, repo.sessionWindowCalls)
			if existingCooldown == "none" {
				require.Equal(t, 1, repo.rateLimitCalls)
				require.True(t, reset.Equal(repo.lastRateLimitReset))
			} else {
				require.Zero(t, repo.rateLimitCalls, "refreshing usage must not shorten an existing cooldown")
			}
		})
	}
}

func TestUpdateSessionWindow_PartialWeeklyUsageWithoutFiveHourStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		utilization string
		wantPercent float64
	}{
		{name: "explicit zero", utilization: "0", wantPercent: 0},
		{name: "missing", wantPercent: 50},
		{name: "invalid", utilization: "invalid", wantPercent: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset := time.Now().Add(72 * time.Hour).Truncate(time.Second)
			account := &Account{
				ID: 11, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
				Extra: map[string]any{
					"passive_usage_7d_utilization":    0.5,
					"passive_usage_7d_reset":          reset.Unix(),
					"passive_usage_7d_oi_utilization": 0.9,
					"passive_usage_7d_oi_reset":       reset.Unix(),
				},
			}
			repo := &anthropicUsageSnapshotRepo{account: account}
			svc := NewRateLimitService(repo, nil, nil, nil, nil)
			headers := http.Header{}
			headers.Set("anthropic-ratelimit-unified-7d-utilization", tc.utilization)
			headers.Set("anthropic-ratelimit-unified-7d-reset", "invalid")
			headers.Set("anthropic-ratelimit-unified-fallback-percentage", "0.5")

			svc.UpdateSessionWindow(t.Context(), account, headers)

			reader := &AccountUsageService{accountRepo: repo, cache: &UsageCache{}, usageLogRepo: &usageBatchLogRepoStub{}}
			usage, err := reader.GetPassiveUsage(t.Context(), account.ID)
			require.NoError(t, err)
			require.NotNil(t, usage.SevenDay)
			require.Equal(t, tc.wantPercent, usage.SevenDay.Utilization)
			require.True(t, reset.Equal(*usage.SevenDay.ResetsAt))
			require.NotNil(t, usage.SevenDayFable)
			require.Equal(t, 90.0, usage.SevenDayFable.Utilization, "an omitted model window must keep its last sample")
			require.Zero(t, repo.sessionWindowCalls)
		})
	}
}
