//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Only persistence is stubbed: the candidate is serialized through the real
// Redis snapshot projection before the production threshold service sees it.
type snapshotThresholdRepo struct {
	service.AccountRepository
	accountPauses int
	modelPauses   int
	until         time.Time
}

func (r *snapshotThresholdRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, _ string) error {
	r.accountPauses++
	r.until = until
	return nil
}

func (r *snapshotThresholdRepo) SetModelRateLimit(_ context.Context, _ int64, _ string, until time.Time, _ ...string) error {
	r.modelPauses++
	r.until = until
	return nil
}

func TestSchedulerCacheAnthropicThresholdAdmission(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(time.Hour)
	cases := []struct {
		name                       string
		fiveHour, sevenDay, fable  float64
		threshold                  int
		expired                    bool
		accountPaused, fablePaused bool
	}{
		{name: "shared 5h", fiveHour: .60, threshold: 60, accountPaused: true},
		{name: "shared 7d", sevenDay: .66, threshold: 60, accountPaused: true},
		{name: "Fable only", sevenDay: .40, fable: .75, threshold: 60, fablePaused: true},
		{name: "below threshold", fiveHour: .59, sevenDay: .59, fable: .59, threshold: 60},
		{name: "expired windows", fiveHour: .75, sevenDay: .75, fable: .75, threshold: 60, expired: true},
		{name: "disabled override", fiveHour: .75, sevenDay: .75, fable: .75, threshold: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			end := reset
			if tc.expired {
				end = now.Add(-time.Hour)
			}
			account := service.Account{
				ID: 3, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
				Status: service.StatusActive, Schedulable: true, SessionWindowEnd: &end,
				Credentials: map[string]any{"account_scheduling_threshold": tc.threshold, "access_token": "test-only-token"},
				Extra: map[string]any{
					"session_window_utilization":   tc.fiveHour,
					"passive_usage_7d_utilization": tc.sevenDay, "passive_usage_7d_reset": end.Unix(),
					"passive_usage_7d_oi_utilization": tc.fable, "passive_usage_7d_oi_reset": end.Unix(),
					"unrelated_large_payload": "drop me",
				},
			}
			cache := newSchedulerCacheUnit(t)
			bucket := service.SchedulerBucket{GroupID: 8, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
			token, err := cache.CaptureBucketWriteToken(ctx, bucket)
			require.NoError(t, err)
			require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{account}))
			candidates, hit, err := cache.GetSnapshot(ctx, bucket)
			require.NoError(t, err)
			require.True(t, hit)
			require.Len(t, candidates, 1)
			require.NotContains(t, candidates[0].Credentials, "access_token")
			require.NotContains(t, candidates[0].Extra, "unrelated_large_payload")
			full, err := cache.GetAccount(ctx, account.ID)
			require.NoError(t, err)
			require.NotNil(t, full)
			for _, candidate := range []*service.Account{candidates[0], full} {
				repo := &snapshotThresholdRepo{}
				limiter := service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
				// Default platform thresholds are disabled, so the per-account override
				// must survive both cache read paths to trigger admission control.
				limiter.SetSettingService(service.NewSettingService(nil, &config.Config{}))
				require.Equal(t, tc.accountPaused, limiter.ApplyAccountSchedulingThreshold(ctx, candidate))
				require.Equal(t, tc.accountPaused, repo.accountPauses == 1)
				require.Equal(t, tc.fablePaused, repo.modelPauses == 1)
				if tc.accountPaused || tc.fablePaused {
					require.True(t, end.Equal(repo.until))
				}
				if tc.fablePaused {
					require.False(t, candidate.IsSchedulableForModel("claude-fable-5"))
					require.True(t, candidate.IsSchedulableForModel("claude-opus-5"))
				}
			}
		})
	}
}

func TestSchedulerCacheAnthropicUsageRefresh(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	end := now.Add(time.Hour)
	cache := newSchedulerCacheUnit(t)
	bucket := service.SchedulerBucket{GroupID: 8, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	account := service.Account{
		ID: 3, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"account_scheduling_threshold": 60},
		Extra:       map[string]any{"passive_usage_7d_utilization": .59, "passive_usage_7d_reset": end.Unix()},
	}
	token, err := cache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(ctx, bucket, token, []service.Account{account}))
	for _, used := range []float64{.59, .66, .10} {
		account.Extra["passive_usage_7d_utilization"] = used
		// UpdateExtra refreshes account payloads without rebuilding bucket membership.
		require.NoError(t, cache.SetAccount(ctx, &account))
		candidates, hit, err := cache.GetSnapshot(ctx, bucket)
		require.NoError(t, err)
		require.True(t, hit)
		require.Len(t, candidates, 1)
		decision := service.EvaluateAccountSchedulingThreshold(candidates[0], map[string]int{service.PlatformAnthropic: 100}, now)
		require.Equal(t, used >= .60, decision.ShouldPause)
	}
}
