//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshotPlatformsPreserveKiroAndGrok(t *testing.T) {
	platforms := schedulerSnapshotPlatforms()

	require.Contains(t, platforms, PlatformKiro)
	require.Contains(t, platforms, PlatformGrok)
	require.Contains(t, platforms, PlatformAdobe)
	require.Equal(t, AllowedQuotaPlatforms, platforms)
}
