package service

import (
	"encoding/json"
	"fmt"
	"strings"
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

func TestBuildUsageInfo_ScopedFableLimits(t *testing.T) {
	resetAt := time.Now().Add(72 * time.Hour).In(time.FixedZone("UTC+8", 8*60*60)).Truncate(time.Microsecond)
	entry := fmt.Sprintf(`{"kind":"weekly_scoped","group":"weekly","percent":5.25,"resets_at":%q,"scope":{"model":{"display_name":"Fable"},"surface":null},"is_active":false}`, resetAt.Format(time.RFC3339Nano))
	for _, tc := range []struct {
		name       string
		limits     string
		legacy     string
		wantWindow bool
		wantUsage  float64
	}{
		{name: "inactive scoped window", limits: entry, wantWindow: true, wantUsage: 5.25},
		{name: "active scoped window", limits: strings.Replace(entry, `"is_active":false`, `"is_active":true`, 1), wantWindow: true, wantUsage: 5.25},
		{name: "omitted surface", limits: strings.Replace(entry, `,"surface":null`, ``, 1), wantWindow: true, wantUsage: 5.25},
		{name: "zero percent", limits: strings.Replace(entry, `"percent":5.25`, `"percent":0`, 1), wantWindow: true},
		{name: "over quota percent", limits: strings.Replace(entry, `"percent":5.25`, `"percent":105.25`, 1), wantWindow: true, wantUsage: 105.25},
		{name: "legacy takes precedence", limits: entry, legacy: fmt.Sprintf(`{"utilization":56,"resets_at":%q}`, resetAt.Format(time.RFC3339Nano)), wantWindow: true, wantUsage: 56},
		{name: "null legacy", limits: entry, legacy: "null", wantWindow: true, wantUsage: 5.25},
		{name: "other model", limits: strings.Replace(entry, `"Fable"`, `"Sonnet"`, 1)},
		{name: "unscoped weekly", limits: strings.Replace(entry, `"weekly_scoped"`, `"weekly"`, 1)},
		{name: "other group", limits: strings.Replace(entry, `"group":"weekly"`, `"group":"daily"`, 1)},
		{name: "missing model", limits: strings.Replace(entry, `{"display_name":"Fable"}`, `null`, 1)},
		{name: "surface restricted", limits: strings.Replace(entry, `"surface":null`, `"surface":{"display_name":"Claude Code"}`, 1)},
		{name: "missing percent", limits: strings.Replace(entry, `"percent":5.25,`, ``, 1)},
		{name: "null percent", limits: strings.Replace(entry, `"percent":5.25`, `"percent":null`, 1)},
		{name: "negative percent", limits: strings.Replace(entry, `"percent":5.25`, `"percent":-1`, 1)},
		{name: "missing reset", limits: strings.Replace(entry, resetAt.Format(time.RFC3339Nano), "", 1)},
		{name: "invalid reset", limits: strings.Replace(entry, resetAt.Format(time.RFC3339Nano), "not-a-time", 1)},
		{name: "invalid entry before valid", limits: strings.Replace(entry, resetAt.Format(time.RFC3339Nano), "not-a-time", 1) + "," + entry, wantWindow: true, wantUsage: 5.25},
		{name: "unknown entry before valid", limits: `null,{"kind":"future_limit"},` + entry, wantWindow: true, wantUsage: 5.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := tc.legacy
			if legacy == "" {
				legacy = "null"
			}
			raw := fmt.Sprintf(`{"five_hour":{"utilization":12},"seven_day":{"utilization":34,"resets_at":%q},"seven_day_overage_included":%s,"limits":[%s]}`, resetAt.Format(time.RFC3339Nano), legacy, tc.limits)
			var resp ClaudeUsageResponse
			require.NoError(t, json.Unmarshal([]byte(raw), &resp))
			now := time.Now()
			info := (&AccountUsageService{}).buildUsageInfo(&resp, &now)
			require.Equal(t, 12.0, info.FiveHour.Utilization)
			require.Equal(t, 34.0, info.SevenDay.Utilization)
			if !tc.wantWindow {
				require.Nil(t, info.SevenDayFable)
				return
			}
			require.NotNil(t, info.SevenDayFable)
			require.Equal(t, tc.wantUsage, info.SevenDayFable.Utilization)
			require.NotNil(t, info.SevenDayFable.ResetsAt)
			require.True(t, resetAt.Equal(*info.SevenDayFable.ResetsAt))
			require.Positive(t, info.SevenDayFable.RemainingSeconds)
		})
	}
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
