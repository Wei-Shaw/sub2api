//go:build unit

package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestQuotaWindowThresholds_CNIndependentWeeklyLimit(t *testing.T) {
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformMiniMax, PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{Platform: platform, Extra: map[string]any{
				"auto_pause_5h_disabled":          true,
				"auto_pause_7d_threshold":         0.90,
				platform + "_5h_used_percent":     100.0,
				platform + "_5h_reset_at":         now.Add(time.Hour).Format(time.RFC3339),
				platform + "_weekly_used_percent": 89.9,
				platform + "_weekly_reset_at":     now.Add(4 * 24 * time.Hour).Format(time.RFC3339),
			}}
			defaults := map[string]int{platform: 100}
			require.False(t, EvaluateAccountSchedulingThreshold(account, defaults, now).ShouldPause)
			account.Extra[platform+"_weekly_used_percent"] = 90.0
			decision := EvaluateAccountSchedulingThreshold(account, defaults, now)
			require.True(t, decision.ShouldPause)
			require.Equal(t, "weekly", decision.Window)
			require.Equal(t, 90.0, decision.ThresholdPercent)
			require.Equal(t, now.Add(4*24*time.Hour), *decision.Until)
			account.Extra["auto_pause_7d_disabled"] = true
			require.False(t, EvaluateAccountSchedulingThreshold(account, map[string]int{platform: 50}, now).ShouldPause)
		})
	}
}

func TestQuotaWindowThresholds_MonthlyResetPrecisionAndInheritance(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Platform: PlatformOpenCodeGo, Extra: map[string]any{
		"auto_pause_monthly_threshold":     0.905,
		"opencode_go_monthly_used_percent": 90.4,
		"opencode_go_monthly_reset_at":     now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}}
	require.False(t, EvaluateAccountSchedulingThreshold(account, nil, now).ShouldPause)
	account.Extra["opencode_go_monthly_used_percent"] = 90.5
	d := EvaluateAccountSchedulingThreshold(account, nil, now)
	require.True(t, d.ShouldPause)
	require.Equal(t, 90.5, d.ThresholdPercent)
	account.Extra["opencode_go_monthly_reset_at"] = now.Format(time.RFC3339)
	require.False(t, EvaluateAccountSchedulingThreshold(account, nil, now).ShouldPause)
	delete(account.Extra, "opencode_go_monthly_reset_at")
	require.False(t, EvaluateAccountSchedulingThreshold(account, nil, now).ShouldPause)
	// Explicit 100 means pause at full usage; legacy all-window 100 stays disabled.
	account.Extra["opencode_go_monthly_reset_at"] = now.Add(time.Hour).Format(time.RFC3339)
	account.Extra["auto_pause_monthly_threshold"] = 1.0
	account.Extra["opencode_go_monthly_used_percent"] = 100.0
	require.True(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenCodeGo: 100}, now).ShouldPause)
	account.Extra["auto_pause_monthly_threshold"] = 0.0
	require.False(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenCodeGo: 100}, now).ShouldPause)
	require.True(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenCodeGo: 80}, now).ShouldPause)
	account.Extra["opencode_go_monthly_used_percent"] = math.Inf(1)
	require.False(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenCodeGo: 80}, now).ShouldPause)
}

func TestQuotaWindowThresholds_ValidationAndSnapshotPreservation(t *testing.T) {
	for _, value := range []any{-1, 90, math.NaN(), math.Inf(1), "invalid", true} {
		require.Error(t, validateAccountQuotaWindowThresholds(map[string]any{"auto_pause_7d_threshold": value}))
	}
	for _, value := range []any{nil, 0, 0.9, 1.0, "0.9"} {
		require.NoError(t, validateAccountQuotaWindowThresholds(map[string]any{"auto_pause_7d_threshold": value}))
	}
	require.Error(t, validateAccountQuotaWindowThresholds(map[string]any{"auto_pause_5h_disabled": "true"}))
	next := map[string]any{"auto_pause_7d_threshold": 0.9, "zhipu_weekly_used_percent": 20, "opencode_go_monthly_used_percent": 1}
	preserveCNQuotaRuntimeExtra(map[string]any{"zhipu_weekly_used_percent": 85}, next)
	require.Equal(t, 85, next["zhipu_weekly_used_percent"])
	require.Equal(t, 0.9, next["auto_pause_7d_threshold"])
	require.NotContains(t, next, "opencode_go_monthly_used_percent")
	require.Equal(t, map[string]any{"zhipu_weekly_used_percent": 85}, preserveCNQuotaRuntimeExtra(map[string]any{"zhipu_weekly_used_percent": 85}, nil))
	// Invalid input is rejected before any repository mutation.
	svc := &adminServiceImpl{}
	_, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{"auto_pause_7d_threshold": 90}})
	require.ErrorContains(t, err, "ratios")
}

func TestQuotaWindowThresholds_PersistCNStopWithoutGlobalPercentage(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	settings := newMockSettingRepo()
	settings.data[SettingKeyAccountSchedulingThresholds] = `{"zhipu":100}`
	repo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settings, &config.Config{}))
	reset := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	account := &Account{ID: 123, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"account_mode": AccountModeCoding},
		Extra:       map[string]any{"auto_pause_7d_threshold": 0.9, "zhipu_weekly_used_percent": 90.0, "zhipu_weekly_reset_at": reset.Format(time.RFC3339)},
	}
	require.True(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Equal(t, 1, repo.tempCalls)
	require.Equal(t, reset, *account.TempUnschedulableUntil)
	require.True(t, IsAccountSchedulingThresholdReason(repo.lastTempReason))
	require.False(t, account.IsSchedulable())
}
