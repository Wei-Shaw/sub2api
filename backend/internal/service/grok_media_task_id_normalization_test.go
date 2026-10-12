package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokMediaVideoRequestSessionHashNormalizesSeedanceTaskKey(t *testing.T) {
	const (
		userID   = int64(10)
		apiKeyID = int64(20)
	)

	// Create binds ownership using the "seedance:" billing key, while
	// status/delete requests carry the raw upstream task ID. Both must derive
	// the same session hash so the lookup hits the create-time binding.
	prefixed := GrokMediaVideoRequestSessionHash(SeedanceTaskKey("task-abc"), userID, apiKeyID)
	raw := GrokMediaVideoRequestSessionHash("task-abc", userID, apiKeyID)
	require.NotEmpty(t, raw)
	require.Equal(t, raw, prefixed)

	// Whitespace-only input still yields an empty hash (no ownership key).
	require.Empty(t, GrokMediaVideoRequestSessionHash("  seedance:  ", userID, apiKeyID))
}
