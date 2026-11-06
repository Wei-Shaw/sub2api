package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupForceOpenAIFastRoundtrip(t *testing.T) {
	groupID := int64(50)
	apiKey := &APIKey{
		ID: 82, UserID: 40, GroupID: &groupID, Key: "sk-fast-roundtrip", Status: StatusActive,
		User: &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "fast-roundtrip", Platform: PlatformOpenAI, Status: StatusActive,
			Hydrated: true, ForceOpenAIFast: true, FreeOpenAIFast: true,
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.Hydrated)
	require.True(t, materialized.Group.ForceOpenAIFast)
	require.True(t, materialized.Group.FreeOpenAIFast)
	require.Equal(t, apiKeyAuthSnapshotVersion, cached.Snapshot.Version)
}

func TestAPIKeyAuthSnapshotImagePriceExpressionsRoundtripAndIsolation(t *testing.T) {
	groupID := int64(51)
	apiKey := &APIKey{
		ID: 83, UserID: 40, GroupID: &groupID, Key: "sk-image-expression-roundtrip", Status: StatusActive,
		User: &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "image-expressions", Platform: PlatformGrok, Status: StatusActive, Hydrated: true,
			ImagePriceExpressions: map[string]string{"grok-imagine-image-2.0": "0.04", "Grok-Imagine-Image-2.0": "0.06"},
		},
	}
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	apiKey.Group.ImagePriceExpressions["grok-imagine-image-2.0"] = "0.99"
	require.Equal(t, "0.04", snapshot.Group.ImagePriceExpressions["grok-imagine-image-2.0"])

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	restored, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, snapshot.Group.ImagePriceExpressions, restored.Group.ImagePriceExpressions)
	restored.Group.ImagePriceExpressions["grok-imagine-image-2.0"] = "0.88"
	second, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, "0.04", second.Group.ImagePriceExpressions["grok-imagine-image-2.0"], "a request must not mutate a shared cached price")
	require.Equal(t, "0.06", second.Group.ImagePriceExpressions["Grok-Imagine-Image-2.0"])

	cached.Snapshot.Version = 25
	stale, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.False(t, used, "pre-expression snapshots must be reloaded")
	require.Nil(t, stale)
}
