//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitService_ApplyAccountSchedulingThreshold_SetsTempUnschedulable(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":80}`

	accountRepo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	until := time.Now().UTC().Add(6 * time.Hour)
	account := &Account{
		ID:          1001,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_7d_used_percent": 91.5,
			"codex_7d_reset_at":     until.Format(time.RFC3339),
		},
	}

	blocked := rl.ApplyAccountSchedulingThreshold(context.Background(), account)

	require.True(t, blocked)
	require.Equal(t, 1, accountRepo.tempCalls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.WithinDuration(t, until, *account.TempUnschedulableUntil, time.Second)
	require.True(t, IsAccountSchedulingThresholdReason(accountRepo.lastTempReason))

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(accountRepo.lastTempReason), &payload))
	require.Equal(t, PlatformOpenAI, payload["platform"])
	require.Equal(t, "7d", payload["window"])
	require.Equal(t, float64(80), payload["threshold_percent"])
	require.Equal(t, float64(91.5), payload["used_percent"])
	require.Contains(t, payload["error_message"], "91.5% used >= 80%")
}

func TestRateLimitService_ApplyAccountSchedulingThreshold_UsesAccountOverrideInReason(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":90}`

	accountRepo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	until := time.Now().UTC().Add(6 * time.Hour)
	account := &Account{
		ID:          1003,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"account_scheduling_threshold": 80,
		},
		Extra: map[string]any{
			"codex_7d_used_percent": 85.5,
			"codex_7d_reset_at":     until.Format(time.RFC3339),
		},
	}

	blocked := rl.ApplyAccountSchedulingThreshold(context.Background(), account)

	require.True(t, blocked)
	require.Equal(t, 1, accountRepo.tempCalls)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(accountRepo.lastTempReason), &payload))
	require.Equal(t, float64(80), payload["threshold_percent"])
	require.Equal(t, float64(85.5), payload["used_percent"])
	require.Contains(t, payload["error_message"], "85.5% used >= 80%")
}

func TestRateLimitService_ApplyAccountSchedulingThreshold_SkipsDuplicateTempUnschedulable(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":80}`

	accountRepo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	until := time.Now().UTC().Add(6 * time.Hour).Truncate(time.Second)
	existingReason := BuildDetailedAccountSchedulingThresholdReason(AccountSchedulingThresholdReasonInput{
		Platform:         PlatformOpenAI,
		Window:           "7d",
		ThresholdPercent: 80,
		UsedPercent:      91.5,
		Until:            until,
		Now:              until.Add(-time.Hour),
	})
	account := &Account{
		ID:                      1002,
		Platform:                PlatformOpenAI,
		Status:                  StatusActive,
		Schedulable:             true,
		TempUnschedulableUntil:  &until,
		TempUnschedulableReason: existingReason,
		Extra: map[string]any{
			"codex_7d_used_percent": 91.5,
			"codex_7d_reset_at":     until.Format(time.RFC3339),
		},
	}

	blocked := rl.ApplyAccountSchedulingThreshold(context.Background(), account)

	require.True(t, blocked)
	require.Equal(t, 0, accountRepo.tempCalls)
	require.Equal(t, existingReason, account.TempUnschedulableReason)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.True(t, until.Equal(*account.TempUnschedulableUntil))
}

func TestRateLimitService_ApplyAccountSchedulingThreshold_UnsupportedPlatformDoesNotBlock(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":80}`

	accountRepo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	account := &Account{
		ID:          2002,
		Platform:    PlatformKiro,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"account_scheduling_threshold": 1,
		},
		Extra: map[string]any{
			"kiro_sched_utilization": 99.0,
			"kiro_sched_reset_at":    time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	blocked := rl.ApplyAccountSchedulingThreshold(context.Background(), account)

	require.False(t, blocked)
	require.Equal(t, 0, accountRepo.tempCalls)
	require.Nil(t, account.TempUnschedulableUntil)
	require.Empty(t, account.TempUnschedulableReason)
}
