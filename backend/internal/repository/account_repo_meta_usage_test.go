package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetaUsageSamplingExtraIsSchedulerNeutral(t *testing.T) {
	keys := []string{
		"meta_usage_window_utilization",
		"meta_usage_window_reset",
		"meta_usage_weekly_utilization",
		"meta_usage_weekly_reset",
		"meta_usage_tier",
		"meta_usage_sampled_at",
	}
	for _, key := range keys {
		require.True(t, isSchedulerNeutralExtraKey(key), key)
	}

	updates := make(map[string]any, len(keys))
	for _, key := range keys {
		updates[key] = 0.5
	}
	require.False(t, shouldEnqueueSchedulerOutboxForExtraUpdates(updates))

	require.True(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{
		"manual_override_reason": "operator pin",
	}))
}
