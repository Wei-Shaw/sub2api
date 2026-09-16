package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaudeUsageResponse_FableWindowDecoding(t *testing.T) {
	t.Run("seven_day_overage_included", func(t *testing.T) {
		raw := `{
  "five_hour": {"utilization": 12.0, "resets_at": "2026-07-03T10:00:00Z"},
  "seven_day": {"utilization": 34.0, "resets_at": "2026-07-08T00:00:00Z"},
  "seven_day_overage_included": {"utilization": 56.0, "resets_at": "2026-07-08T03:00:00Z"}
}`
		var resp ClaudeUsageResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))
		require.Equal(t, 56.0, resp.SevenDayOverageIncluded.Utilization)
		require.Equal(t, "2026-07-08T03:00:00Z", resp.SevenDayOverageIncluded.ResetsAt)
	})

	t.Run("absent", func(t *testing.T) {
		raw := `{"five_hour": {"utilization": 12.0, "resets_at": "2026-07-03T10:00:00Z"}}`
		var resp ClaudeUsageResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))
		require.Zero(t, resp.SevenDayOverageIncluded.Utilization)
		require.Empty(t, resp.SevenDayOverageIncluded.ResetsAt)
	})
}

func TestBuildUsageInfo_SevenDayFable(t *testing.T) {
	svc := &AccountUsageService{}
	now := time.Now()

	resetAt := now.Add(72 * time.Hour).UTC().Truncate(time.Second)
	var resp ClaudeUsageResponse
	resp.FiveHour.Utilization = 10
	resp.SevenDayOverageIncluded = ClaudeUsageWindow{
		Utilization: 88,
		ResetsAt:    resetAt.Format(time.RFC3339),
	}

	info := svc.buildUsageInfo(&resp, &now)
	require.NotNil(t, info.SevenDayFable)
	require.Equal(t, 88.0, info.SevenDayFable.Utilization)
	require.NotNil(t, info.SevenDayFable.ResetsAt)
	require.True(t, info.SevenDayFable.ResetsAt.Equal(resetAt))
	require.Greater(t, info.SevenDayFable.RemainingSeconds, 0)

	// 无 Fable 数据时不应创建窗口
	var empty ClaudeUsageResponse
	empty.FiveHour.Utilization = 10
	info = svc.buildUsageInfo(&empty, &now)
	require.Nil(t, info.SevenDayFable)
}

func TestBuildPassiveUsageWindow(t *testing.T) {
	future := time.Now().Add(48 * time.Hour).Unix()

	t.Run("utilization and reset", func(t *testing.T) {
		window := buildPassiveUsageWindow(map[string]any{
			"passive_usage_7d_oi_utilization": 0.87,
			"passive_usage_7d_oi_reset":       float64(future),
		}, "passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset")
		require.NotNil(t, window)
		require.InDelta(t, 87.0, window.Utilization, 1e-9)
		require.NotNil(t, window.ResetsAt)
		require.Equal(t, future, window.ResetsAt.Unix())
		require.Greater(t, window.RemainingSeconds, 0)
	})

	t.Run("no data returns nil", func(t *testing.T) {
		require.Nil(t, buildPassiveUsageWindow(nil, "u", "r"))
		require.Nil(t, buildPassiveUsageWindow(map[string]any{}, "u", "r"))
	})

	t.Run("expired reset clamps remaining to zero", func(t *testing.T) {
		past := time.Now().Add(-time.Hour).Unix()
		window := buildPassiveUsageWindow(map[string]any{
			"u": 0.5,
			"r": float64(past),
		}, "u", "r")
		require.NotNil(t, window)
		require.Equal(t, 0, window.RemainingSeconds)
	})

	t.Run("utilization only", func(t *testing.T) {
		window := buildPassiveUsageWindow(map[string]any{"u": 0.25}, "u", "r")
		require.NotNil(t, window)
		require.InDelta(t, 25.0, window.Utilization, 1e-9)
		require.Nil(t, window.ResetsAt)
	})
}

func TestSyncActiveToPassive_WritesFableExtras(t *testing.T) {
	repo := &accountUsageCodexProbeRepo{updateExtraCh: make(chan map[string]any, 1)}
	svc := &AccountUsageService{accountRepo: repo}

	resetAt := time.Now().Add(72 * time.Hour).Truncate(time.Second)
	usage := &UsageInfo{
		SevenDayFable: &UsageProgress{
			Utilization: 87,
			ResetsAt:    &resetAt,
		},
	}

	svc.syncActiveToPassive(t.Context(), 1, usage)

	select {
	case updates := <-repo.updateExtraCh:
		require.InDelta(t, 0.87, updates["passive_usage_7d_oi_utilization"], 1e-9)
		require.Equal(t, resetAt.Unix(), updates["passive_usage_7d_oi_reset"])
		require.Contains(t, updates, "passive_usage_sampled_at")
	default:
		t.Fatal("expected UpdateExtra to be called with fable extras")
	}
}
