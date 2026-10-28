package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIKeyAuthSnapshotClientExportRoundtrip preserves selected client models
// through actual snapshot serialization and the normal cache restoration path.
func TestAPIKeyAuthSnapshotClientExportRoundtrip(t *testing.T) {
	groupID := int64(60)
	cfg := GroupClientExportConfig{CodexModel: "gpt-5.6-sol", GeminiModel: "gemini-2.5-flash", CodexAuthMode: "api-key", CodexReasoningEffort: "high", CodexCatalogMode: "file", CCUsageMode: "disabled", CCUsageInterval: "15"}
	key := &APIKey{ID: 92, UserID: 46, GroupID: &groupID, Status: StatusActive,
		User:  &User{ID: 46, Status: StatusActive},
		Group: &Group{ID: groupID, Hydrated: true, ClientExportConfig: cfg}}
	svc := &APIKeyService{}
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), key)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))
	restored, used, err := svc.applyAuthCacheEntry("local-test-key", &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.Equal(t, cfg, restored.Group.ClientExportConfig)
}
