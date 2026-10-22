package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupSchedulerConfigMapPersistsSelectionMode(t *testing.T) {
	payload := groupSchedulerConfigMap(service.GroupSchedulerConfig{
		Strategy:      "high_availability",
		SelectionMode: "strict_health",
	})

	require.Equal(t, "high_availability", payload["strategy"])
	require.Equal(t, "strict_health", payload["selection_mode"])
}
