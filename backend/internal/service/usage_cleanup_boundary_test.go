package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageCleanupBoundaryPersistenceAndValidation(t *testing.T) {
	var legacy UsageCleanupFilters
	require.NoError(t, json.Unmarshal([]byte(`{"start_time":"2026-09-10T05:00:00Z","end_time":"2026-09-10T05:30:00Z"}`), &legacy))
	require.False(t, legacy.EndExclusive)
	current := legacy
	current.EndExclusive = true
	raw, err := json.Marshal(current)
	require.NoError(t, err)
	var restored UsageCleanupFilters
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.True(t, restored.EndExclusive)
	require.Equal(t, current.EndTime, restored.EndTime)
	s := NewUsageCleanupService(nil, nil, nil, nil)
	require.NoError(t, s.validateFilters(restored))
	restored.EndTime = restored.StartTime
	require.Error(t, s.validateFilters(restored))
	restored.EndTime = restored.StartTime.Add(time.Minute)
	require.NoError(t, s.validateFilters(restored))
}
