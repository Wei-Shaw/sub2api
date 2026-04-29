//go:build unit

package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJitteredRectifierSettingsTTL_WithinRange(t *testing.T) {
	min := rectifierSettingsBaseTTL - rectifierSettingsJitter
	max := rectifierSettingsBaseTTL + rectifierSettingsJitter

	// 跑 2000 次看分布——rand/v2 的 Int64N 在 [0, 2*jitter] 理论上均匀，
	// 抽样量够大时三块 bucket（低/中/高）都应有 value，否则 offset 计算有 bug。
	var countLow, countMid, countHigh int
	for i := 0; i < 2000; i++ {
		ttl := jitteredRectifierSettingsTTL()
		require.GreaterOrEqual(t, ttl, min, "TTL 必须 ≥ base-jitter")
		require.LessOrEqual(t, ttl, max, "TTL 必须 ≤ base+jitter")

		switch {
		case ttl < rectifierSettingsBaseTTL-5*time.Second:
			countLow++
		case ttl > rectifierSettingsBaseTTL+5*time.Second:
			countHigh++
		default:
			countMid++
		}
	}
	require.Greater(t, countLow, 100, "expected jitter to produce some low-TTL samples")
	require.Greater(t, countMid, 100, "expected jitter to produce some mid-TTL samples")
	require.Greater(t, countHigh, 100, "expected jitter to produce some high-TTL samples")
}
