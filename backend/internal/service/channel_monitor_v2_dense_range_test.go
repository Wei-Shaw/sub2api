//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// denseRangeTestNow is a fixed clock whose minute is not aligned to the hour, so
// bucket alignment is observable in the assertions below.
var denseRangeTestNow = time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)

func newDenseRangeTestService() *ChannelMonitorV2Service {
	return &ChannelMonitorV2Service{now: func() time.Time { return denseRangeTestNow }}
}

// TestChannelMonitorV2ParseFilterDenseRangeTokens covers the six whitelisted
// <window>-<bucket> tokens. Each one must resolve to its own window/bucket and
// fall through the same start/end computation as the coarse ranges: buckets
// larger than 1m snap to the next whole bucket, 1m buckets keep the raw
// now-centred window (the alignment branch is `bucket > time.Minute`).
func TestChannelMonitorV2ParseFilterDenseRangeTokens(t *testing.T) {
	svc := newDenseRangeTestService()

	cases := []struct {
		token  string
		window time.Duration
		bucket time.Duration
	}{
		{token: "30m-1m", window: 30 * time.Minute, bucket: time.Minute},
		{token: "1h-1m", window: time.Hour, bucket: time.Minute},
		{token: "12h-5m", window: 12 * time.Hour, bucket: 5 * time.Minute},
		{token: "24h-5m", window: 24 * time.Hour, bucket: 5 * time.Minute},
		{token: "7d-1h", window: 7 * 24 * time.Hour, bucket: time.Hour},
		{token: "30d-12h", window: 30 * 24 * time.Hour, bucket: 12 * time.Hour},
	}
	require.Len(t, channelMonitorV2DenseRangeTokens, len(cases),
		"dense range whitelist and this table must stay in sync")

	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			filter, err := svc.ParseFilter(tc.token, nil, nil, nil)
			require.NoError(t, err)
			require.Equal(t, tc.token, filter.Range)
			require.Equal(t, tc.bucket, filter.Bucket)

			if tc.bucket > time.Minute {
				wantEnd := denseRangeTestNow.Truncate(tc.bucket).Add(tc.bucket)
				require.Equal(t, wantEnd, filter.End)
				require.Equal(t, wantEnd.Add(-tc.window), filter.Start)
				return
			}
			// 1m buckets must NOT enter the alignment branch.
			require.Equal(t, denseRangeTestNow, filter.End)
			require.Equal(t, denseRangeTestNow.Add(-tc.window), filter.Start)
		})
	}
}

// TestChannelMonitorV2ParseFilterDenseRangeTrimsSurroundingSpace mirrors the
// switch's `strings.TrimSpace(rangeValue)`: padding is tolerated and the filter
// carries the canonical token.
func TestChannelMonitorV2ParseFilterDenseRangeTrimsSurroundingSpace(t *testing.T) {
	svc := newDenseRangeTestService()

	filter, err := svc.ParseFilter("  24h-5m\t", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "24h-5m", filter.Range)
	require.Equal(t, 5*time.Minute, filter.Bucket)
}

// TestChannelMonitorV2ParseFilterDenseRangeRejectsUnknownTokens guards the
// explicit whitelist: no generic <n><unit>-<n><unit> parsing is allowed, so
// near-misses and absurd combinations stay rejected with the original error.
func TestChannelMonitorV2ParseFilterDenseRangeRejectsUnknownTokens(t *testing.T) {
	svc := newDenseRangeTestService()

	invalid := []string{
		"30m-7s",   // unlisted bucket unit
		"24h-99m",  // unlisted bucket length
		"9999d-1s", // absurd combination
		"12h-1m",   // bucket not whitelisted for this window
		"30d-1h",
		"7d-5m",
		"30M-1M", // case-sensitive: uppercase is not a token
		"1h-1h",
		"90m-1m", // coarse windows are not dense tokens
		"24h-24h",
	}

	for _, token := range invalid {
		t.Run(token, func(t *testing.T) {
			_, err := svc.ParseFilter(token, nil, nil, nil)
			require.ErrorIs(t, err, ErrChannelMonitorV2InvalidRange)
		})
	}
}

// TestChannelMonitorV2ParseFilterCoarseRangesUnchanged is the regression guard
// for the pre-existing switch branches. The dense tokens are purely additive:
// "", 90m, 24h, 7d and 30d must keep producing byte-identical filters
// (range label, bucket, aligned start/end).
func TestChannelMonitorV2ParseFilterCoarseRangesUnchanged(t *testing.T) {
	svc := newDenseRangeTestService()

	cases := []struct {
		input     string
		wantRange string
		window    time.Duration
		bucket    time.Duration
	}{
		{input: "", wantRange: "90m", window: 90 * time.Minute, bucket: 5 * time.Minute},
		{input: "90m", wantRange: "90m", window: 90 * time.Minute, bucket: 5 * time.Minute},
		{input: "24h", wantRange: "24h", window: 24 * time.Hour, bucket: time.Hour},
		{input: "7d", wantRange: "7d", window: 7 * 24 * time.Hour, bucket: 12 * time.Hour},
		{input: "30d", wantRange: "30d", window: 30 * 24 * time.Hour, bucket: 24 * time.Hour},
	}

	for _, tc := range cases {
		name := tc.input
		if name == "" {
			name = "<empty>"
		}
		t.Run(name, func(t *testing.T) {
			filter, err := svc.ParseFilter(tc.input, nil, nil, nil)
			require.NoError(t, err)
			require.Equal(t, tc.wantRange, filter.Range)
			require.Equal(t, tc.bucket, filter.Bucket)

			wantEnd := denseRangeTestNow.Truncate(tc.bucket).Add(tc.bucket)
			require.Equal(t, wantEnd, filter.End)
			require.Equal(t, wantEnd.Add(-tc.window), filter.Start)
		})
	}

	// Still rejected exactly as before.
	_, err := svc.ParseFilter("15d", nil, nil, nil)
	require.ErrorIs(t, err, ErrChannelMonitorV2InvalidRange)
}
