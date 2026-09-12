package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaHeadroomFactorUsesCanonicalWindows(t *testing.T) {
	now := time.Now()
	shortMins, longMins := 300, 10080
	shortReset, longReset := 3600, 86400
	shortUsed, longUsed := 96.0, 25.0
	for _, primaryIs5h := range []bool{true, false} {
		name := "primary_is_7d"
		snapshot := &OpenAICodexUsageSnapshot{
			PrimaryUsedPercent: &longUsed, PrimaryWindowMinutes: &longMins, PrimaryResetAfterSeconds: &longReset,
			SecondaryUsedPercent: &shortUsed, SecondaryWindowMinutes: &shortMins, SecondaryResetAfterSeconds: &shortReset,
		}
		if primaryIs5h {
			name = "primary_is_5h"
			snapshot = &OpenAICodexUsageSnapshot{
				PrimaryUsedPercent: &shortUsed, PrimaryWindowMinutes: &shortMins, PrimaryResetAfterSeconds: &shortReset,
				SecondaryUsedPercent: &longUsed, SecondaryWindowMinutes: &longMins, SecondaryResetAfterSeconds: &longReset,
			}
		}
		t.Run(name, func(t *testing.T) {
			for _, rawOnly := range []bool{false, true} {
				fields := "canonical_and_raw"
				if rawOnly {
					fields = "raw_only"
				}
				t.Run(fields, func(t *testing.T) {
					extra := buildCodexUsageExtraUpdates(snapshot, now)
					if rawOnly {
						for key := range extra {
							if strings.HasPrefix(key, "codex_5h_") || strings.HasPrefix(key, "codex_7d_") {
								delete(extra, key)
							}
						}
					}
					// 周余量 75%，5h 余量低于 10% 时沿用现有的减半规则。
					require.InDelta(t, 0.375, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now), 1e-9)
				})
			}
		})
	}
}

func TestOpenAIQuotaHeadroomFactorCanonicalPrecedence(t *testing.T) {
	now := time.Now()
	extra := map[string]any{
		"codex_primary_used_percent":   40.0,
		"codex_secondary_used_percent": 20.0,
		"codex_usage_updated_at":       now.Format(time.RFC3339),
	}
	// 没有窗口长度的历史数据仍沿用 primary=7d、secondary=5h。
	require.InDelta(t, 0.6, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now), 1e-9)
	extra["codex_7d_used_percent"] = 80.0
	extra["codex_5h_used_percent"] = 10.0
	require.InDelta(t, 0.2, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now), 1e-9)

	// 规范窗口有效时，不应被另一个原始窗口的过期时间误判为已重置。
	extra["codex_7d_reset_at"] = now.Add(time.Hour).Format(time.RFC3339)
	extra["codex_primary_reset_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	require.InDelta(t, 0.2, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now), 1e-9)
	extra["codex_7d_reset_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now))
}

func TestOpenAIQuotaHeadroomFactorRawWindowReset(t *testing.T) {
	now := time.Now()
	extra := map[string]any{
		"codex_primary_used_percent":   96.0,
		"codex_primary_window_minutes": 300,
		"codex_primary_reset_at":       now.Add(-time.Minute).Format(time.RFC3339),
		"codex_secondary_used_percent": 25.0,
		"codex_secondary_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
		"codex_usage_updated_at":       now.Add(-time.Hour).Format(time.RFC3339),
	}
	require.InDelta(t, 0.75, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now), 1e-9)
	extra["codex_secondary_reset_at"] = now.Add(-time.Minute).Format(time.RFC3339)
	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(&Account{Extra: extra}, now))
}

func TestOpenAISchedulerCanonicalResetScoresMatchSnapshot(t *testing.T) {
	now := time.Now()
	scheduler := openAIResetTestScheduler(1)
	accounts := []*Account{
		{ID: 1, Extra: map[string]any{"codex_5h_reset_at": now.Add(10 * time.Minute).Format(time.RFC3339)}},
		{ID: 2, Extra: map[string]any{"codex_5h_reset_at": now.Add(4 * time.Hour).Format(time.RFC3339)}},
	}
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, accounts, nil)
	scores := openAIPlanScores(plan)
	require.InDelta(t, 1, scores[1]-scores[2], 1e-9)

	snapshots := buildOpenAIAccountSchedulerScoreSnapshot(accounts, nil, scheduler.service.openAIWSSchedulerWeights(), false, defaultOpenAIOAuthSchedulingRateMultiplier)
	for _, account := range accounts {
		require.InDelta(t, scores[account.ID], snapshots[account.ID].BaseScore, 1e-9)
	}
}

func TestOpenAISchedulingResetWindowEnd(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	updated := now.Add(-30 * time.Minute)
	account := &Account{Extra: map[string]any{
		"codex_usage_updated_at":       updated.Format(time.RFC3339),
		"codex_5h_reset_after_seconds": 3600,
	}}
	end, ok := openAISchedulingResetWindowEnd(account, now)
	require.True(t, ok)
	require.Equal(t, updated.Add(time.Hour), end)

	account.Extra["codex_5h_reset_at"] = now.Add(10 * time.Minute).Format(time.RFC3339)
	end, ok = openAISchedulingResetWindowEnd(account, now)
	require.True(t, ok)
	require.Equal(t, now.Add(10*time.Minute), end)

	delete(account.Extra, "codex_5h_reset_at")
	account.Extra["codex_5h_reset_after_seconds"] = 60
	_, ok = openAISchedulingResetWindowEnd(account, now)
	require.False(t, ok)
	account.Extra["codex_5h_reset_after_seconds"] = 3600
	delete(account.Extra, "codex_usage_updated_at")
	_, ok = openAISchedulingResetWindowEnd(account, now)
	require.False(t, ok, "缺少采样时间时不能把相对倒计时锚定到当前时间")
	account.Extra["codex_usage_updated_at"] = "invalid"
	_, ok = openAISchedulingResetWindowEnd(account, now)
	require.False(t, ok)

	sessionEnd := now.Add(2 * time.Hour)
	account.SessionWindowEnd = &sessionEnd
	end, ok = openAISchedulingResetWindowEnd(account, now)
	require.True(t, ok)
	require.Equal(t, sessionEnd, end)
	_, ok = openAISchedulingResetWindowEnd(nil, now)
	require.False(t, ok)
}
