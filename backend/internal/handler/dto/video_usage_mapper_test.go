//go:build unit

package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromServiceUserMapsVideoBillingDimensions(t *testing.T) {
	resolution := "1080p"
	duration := 17
	got := usageLogFromServiceUser(&service.UsageLog{
		VideoCount:           1,
		VideoResolution:      &resolution,
		VideoDurationSeconds: &duration,
	})

	require.Equal(t, 1, got.VideoCount)
	require.Equal(t, &resolution, got.VideoResolution)
	require.Equal(t, &duration, got.VideoDurationSeconds)
}
