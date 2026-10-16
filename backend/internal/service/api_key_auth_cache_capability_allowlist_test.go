//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 能力白名单必须在认证快照 JSON 往返后完整保留，否则请求准入与 manifest 投影
// 在缓存命中路径上会静默失效。
func TestAPIKeyAuthSnapshotGroupCapabilityAllowlistRoundtrip(t *testing.T) {
	groupID := int64(61)
	apiKey := &APIKey{
		ID: 93, UserID: 47, GroupID: &groupID, Key: "sk-capability-allowlist-roundtrip", Status: StatusActive,
		User: &User{ID: 47, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "capability-allowlist-roundtrip", Platform: PlatformOpenAI, Status: StatusActive,
			Hydrated: true,
			CapabilityAllowlist: GroupCapabilityAllowlist{
				Enabled:       true,
				CyberPrograms: []string{"daybreak_blue"},
				ServiceTiers:  []string{"priority", "ultrafast"},
			},
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
	require.Equal(t, apiKey.Group.CapabilityAllowlist, materialized.Group.CapabilityAllowlist)
	require.Equal(t, apiKeyAuthSnapshotVersion, cached.Snapshot.Version)
	require.Equal(t, 26, apiKeyAuthSnapshotVersion)
}

func TestAPIKeyService_RejectsAuthSnapshotsBeforeCapabilityAllowlist(t *testing.T) {
	svc := &APIKeyService{}
	for _, version := range []int{24, 25} {
		apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-capability-allowlist", &APIKeyAuthCacheEntry{
			Snapshot: &APIKeyAuthSnapshot{Version: version},
		})
		require.NoError(t, err)
		require.False(t, ok, "version %d", version)
		require.Nil(t, apiKey)
	}
}
