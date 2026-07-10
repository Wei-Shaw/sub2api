package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseResultURLExpiryFromAmzQuery(t *testing.T) {
	t.Parallel()
	raw := "https://cdn.example.com/out.mp4?X-Amz-Date=20260709T010000Z&X-Amz-Expires=3600&X-Amz-Signature=abc"
	got, source := ParseResultURLExpiry(raw, nil)
	require.Equal(t, ResultURLExpirySourceURLQuery, source)
	require.NotNil(t, got)
	require.Equal(t, time.Date(2026, 7, 9, 2, 0, 0, 0, time.UTC), got.UTC())
}

func TestParseResultURLExpiryEstimatedFromCompletedAt(t *testing.T) {
	t.Parallel()
	completed := time.Date(2026, 7, 9, 1, 0, 0, 0, time.UTC)
	got, source := ParseResultURLExpiry("https://cdn.example.com/out.mp4", &completed)
	require.Equal(t, ResultURLExpirySourceEstimated, source)
	require.NotNil(t, got)
	require.Equal(t, completed.Add(24*time.Hour), got.UTC())
}

func TestParseResultURLExpiryUnknownWithoutURL(t *testing.T) {
	t.Parallel()
	got, source := ParseResultURLExpiry("", nil)
	require.Equal(t, ResultURLExpirySourceUnknown, source)
	require.Nil(t, got)
}

func TestReliabilityRemoteAssetAvailableMatchesHandlerExpirySemantics(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	completed := now.Add(-48 * time.Hour)
	expiredSigned := "https://cdn.example.com/out.mp4?X-Amz-Date=20260709T010000Z&X-Amz-Expires=3600&X-Amz-Signature=abc"
	freshSigned := "https://cdn.example.com/out.mp4?X-Amz-Date=20260710T110000Z&X-Amz-Expires=7200&X-Amz-Signature=abc"

	require.False(t, ReliabilityRemoteAssetAvailable("", nil, now))
	require.False(t, ReliabilityRemoteAssetAvailable(expiredSigned, nil, now))
	require.False(t, ReliabilityRemoteAssetAvailable("https://cdn.example.com/out.mp4", &completed, now))
	require.True(t, ReliabilityRemoteAssetAvailable(freshSigned, nil, now))
	require.True(t, ReliabilityRemoteAssetAvailable("https://cdn.example.com/out.mp4", nil, now), "unknown expiry remains allowed remote fallback")
}
