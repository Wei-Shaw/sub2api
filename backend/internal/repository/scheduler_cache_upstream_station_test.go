package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildSchedulerMetadataAccountPreservesUpstreamStationCostFields(t *testing.T) {
	t.Parallel()

	metadata := buildSchedulerMetadataAccount(service.Account{
		ID: 1,
		Extra: map[string]any{
			"managed_by":                "upstream_station_pool",
			"upstream_station_id":       int64(7),
			"upstream_route_id":         int64(9),
			"upstream_effective_rate":   0.4,
			"upstream_remote_group":     "pro",
			"unrelated_sensitive_value": "drop-me",
		},
	})

	require.Equal(t, "upstream_station_pool", metadata.Extra["managed_by"])
	require.Equal(t, 0.4, metadata.Extra["upstream_effective_rate"])
	require.Equal(t, int64(7), metadata.Extra["upstream_station_id"])
	require.Equal(t, int64(9), metadata.Extra["upstream_route_id"])
	require.Equal(t, "pro", metadata.Extra["upstream_remote_group"])
	require.NotContains(t, metadata.Extra, "unrelated_sensitive_value")
}
