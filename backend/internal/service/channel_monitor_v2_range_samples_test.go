package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2PerRangeMinimumSamples(t *testing.T) {
	var thresholds ChannelMonitorV2HealthThresholds
	require.NoError(t, json.Unmarshal([]byte(`{"minimum_sample":30,"minimum_sample_by_range":{"90m":10,"24h":30,"7d":50,"30d":100}}`), &thresholds))
	thresholds = NormalizeChannelMonitorV2HealthThresholds(thresholds)
	for _, tt := range []struct {
		window  string
		minimum int64
	}{{"90m", 10}, {"24h", 30}, {"7d", 50}, {"30d", 100}} {
		t.Run(tt.window, func(t *testing.T) {
			selected := thresholds.ForRange(tt.window)
			p50 := int64(1000)
			metric := ChannelMonitorV2Metric{RequestCount: tt.minimum - 1, CacheRate: 1, CacheRateDenominator: 100000, TTFT: ChannelMonitorV2Latency{SampleCount: tt.minimum - 1, P50Ms: &p50}}
			health := ChannelMonitorV2HealthForWithThresholds(metric, selected)
			require.Equal(t, tt.minimum, health.MinimumSample)
			require.Nil(t, health.Score, "token count must not bypass the request minimum")
			require.Equal(t, "unknown", health.Overall)
			metric.RequestCount = tt.minimum
			metric.TTFT.SampleCount = tt.minimum
			health = ChannelMonitorV2HealthForWithThresholds(metric, selected)
			require.NotNil(t, health.Score)
			require.Equal(t, "healthy", health.Overall)
		})
	}
	// Switching views must never overwrite the saved global fallback or another view.
	require.Equal(t, int64(30), thresholds.MinimumSample)
	require.Equal(t, int64(30), thresholds.ForRange("unknown").MinimumSample)
	encoded, err := json.Marshal(thresholds)
	require.NoError(t, err)
	var saved ChannelMonitorV2HealthThresholds
	require.NoError(t, json.Unmarshal(encoded, &saved))
	require.Equal(t, thresholds.MinimumSampleByRange, saved.MinimumSampleByRange)
}

func TestChannelMonitorV2PerRangeCompatibilityAndBounds(t *testing.T) {
	legacy := NormalizeChannelMonitorV2HealthThresholds(ChannelMonitorV2HealthThresholds{MinimumSample: 30})
	require.Equal(t, int64(30), legacy.ForRange("90m").MinimumSample)
	input := ChannelMonitorV2HealthThresholds{MinimumSample: 30, MinimumSampleByRange: map[string]int64{"90m": 0, "24h": 10001, "unsupported": 1}}
	normalized := NormalizeChannelMonitorV2HealthThresholds(input)
	require.Equal(t, int64(30), normalized.ForRange("90m").MinimumSample)
	require.Equal(t, int64(10000), normalized.ForRange("24h").MinimumSample)
	require.NotContains(t, normalized.MinimumSampleByRange, "unsupported")
	require.Equal(t, int64(0), input.MinimumSampleByRange["90m"], "normalization must not mutate cached configuration")
}
