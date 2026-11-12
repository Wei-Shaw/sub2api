package service

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// applyExtraCalls applies UpdateExtra calls the way the JSONB merge does:
// a nil value is stored as JSON null.
func applyExtraCalls(extra map[string]any, calls []ueCall) map[string]any {
	merged := maps.Clone(extra)
	for _, call := range calls {
		maps.Copy(merged, call.Updates)
	}
	return merged
}

// A Fable-exhausted account stops receiving Fable requests, so its non-Fable
// responses carry 5h and 7d headers but no 7d_oi. A 5h rollover must keep the
// still-current 7d F reading instead of erasing it until the weekly reset.
func TestUpdateSessionWindow_RolloverKeepsCurrentWeeklyReadings(t *testing.T) {
	now := time.Now()
	weeklyReset := now.Add(72 * time.Hour).Unix()
	expiredEnd := now.Add(-time.Minute)
	extra := map[string]any{
		"session_window_utilization":      1.0,
		"passive_usage_7d_utilization":    0.7,
		"passive_usage_7d_reset":          float64(weeklyReset), // as decoded from JSONB
		"passive_usage_7d_oi_utilization": 1.0,
		"passive_usage_7d_oi_reset":       json.Number(strconv.FormatInt(weeklyReset, 10)),
		"passive_usage_sampled_at":        now.Add(-time.Hour).UTC().Format(time.RFC3339),
	}
	account := &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeSetupToken, SessionWindowEnd: &expiredEnd, Extra: extra}

	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(now.Add(5*time.Hour).Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.05")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "0.71")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(weeklyReset, 10))

	repo := &sessionWindowMockRepo{}
	newRateLimitServiceForTest(repo).UpdateSessionWindow(context.Background(), account, headers)

	require.Len(t, repo.updateExtraCalls, 2, "rollover clear, then the new sample")
	require.Equal(t, map[string]any{"session_window_utilization": nil}, repo.updateExtraCalls[0].Updates)

	merged := applyExtraCalls(extra, repo.updateExtraCalls)
	require.Equal(t, 0.05, merged["session_window_utilization"])
	require.Equal(t, 0.71, merged["passive_usage_7d_utilization"])
	require.Equal(t, 1.0, merged["passive_usage_7d_oi_utilization"], "7d F reading must survive a non-Fable rollover")
	require.Equal(t, json.Number(strconv.FormatInt(weeklyReset, 10)), merged["passive_usage_7d_oi_reset"])
	require.NotNil(t, merged["passive_usage_sampled_at"])

	fable := buildPassiveUsageWindow(merged, "passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset")
	require.NotNil(t, fable)
	require.Equal(t, 100.0, fable.Utilization)
}

func TestRolloverPassiveUsageClears(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour).Unix()
	past := now.Add(-time.Hour).Unix()
	weekly := []string{"passive_usage_7d_utilization", "passive_usage_7d_reset"}
	fable := []string{"passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset"}

	for _, tc := range []struct {
		name        string
		extra       map[string]any
		cleared     []string
		kept        []string
		clearSample bool
	}{
		{
			name:  "both current",
			extra: map[string]any{"passive_usage_7d_reset": future, "passive_usage_7d_oi_reset": float64(future)},
			kept:  append(weekly, fable...),
		},
		{
			name:    "weekly expired, fable current",
			extra:   map[string]any{"passive_usage_7d_reset": past, "passive_usage_7d_oi_reset": strconv.FormatInt(future, 10)},
			cleared: weekly,
			kept:    fable,
		},
		{
			name:        "reading without reset is stale",
			extra:       map[string]any{"passive_usage_7d_utilization": 0.4, "passive_usage_7d_oi_reset": nil},
			cleared:     append(weekly, fable...),
			clearSample: true,
		},
		{
			name:        "no extra",
			cleared:     append(weekly, fable...),
			clearSample: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updates := rolloverPassiveUsageClears(tc.extra, now)
			require.Contains(t, updates, "session_window_utilization")
			require.Nil(t, updates["session_window_utilization"])
			for _, key := range tc.cleared {
				value, present := updates[key]
				require.True(t, present, key)
				require.Nil(t, value, key)
			}
			for _, key := range tc.kept {
				require.NotContains(t, updates, key)
			}
			if tc.clearSample {
				require.Contains(t, updates, "passive_usage_sampled_at")
			} else {
				require.NotContains(t, updates, "passive_usage_sampled_at")
			}
		})
	}
}
