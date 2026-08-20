package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHasSyncedUpstreamModel_MissingSnapshotDoesNotRestrict(t *testing.T) {
	account := &Account{Platform: PlatformAntigravity}
	require.True(t, account.HasSyncedUpstreamModel("gemini-3.7-flash"))
}

func TestHasSyncedUpstreamModel_UsesLastSuccessfulList(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Extra:    ApplyUpstreamModelSnapshot(nil, []string{"gemini-3.6-flash-tiered", "gemini-3.7-flash"}, time.Unix(1, 0).UTC()),
		Credentials: map[string]any{
			CredentialKeyModelMappingRestricts: false,
			"model_mapping": map[string]any{
				"gemini-3.6-flash": "gemini-3.6-flash-tiered",
			},
		},
	}

	require.True(t, account.HasSyncedUpstreamModel("gemini-3.7-flash"))
	require.True(t, account.HasSyncedUpstreamModel("gemini-3.6-flash"))
	require.False(t, account.HasSyncedUpstreamModel("claude-unknown-model"))
}

func TestApplyUpstreamModelSnapshot_IgnoresEmptyResult(t *testing.T) {
	extra := map[string]any{"keep": true}
	got := ApplyUpstreamModelSnapshot(extra, nil, time.Time{})
	require.Equal(t, extra, got)
	require.Nil(t, ParseUpstreamModelSnapshot(got))
}

func TestParseUpstreamModelSnapshot_FromJSONMap(t *testing.T) {
	snapshot := ParseUpstreamModelSnapshot(map[string]any{
		UpstreamModelSnapshotExtraKey: map[string]any{
			"models":    []any{"gemini-3.7-flash", " gemini-3.7-flash ", ""},
			"synced_at": "2026-08-19T00:00:00Z",
		},
	})
	require.NotNil(t, snapshot)
	require.Equal(t, []string{"gemini-3.7-flash"}, snapshot.Models)
	require.Equal(t, "2026-08-19T00:00:00Z", snapshot.SyncedAt)
}

func TestSnapshotCoversModel_MissingSnapshotIsNotCovered(t *testing.T) {
	account := &Account{Platform: PlatformAntigravity}
	require.False(t, account.SnapshotCoversModel("gemini-3.7-flash"))
}

func TestSnapshotCoversModel_UsesMapping(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Extra:    ApplyUpstreamModelSnapshot(nil, []string{"gemini-3.6-flash-tiered"}, time.Unix(1, 0).UTC()),
		Credentials: map[string]any{
			CredentialKeyModelMappingRestricts: false,
			"model_mapping": map[string]any{
				"gemini-3.6-flash": "gemini-3.6-flash-tiered",
			},
		},
	}
	require.True(t, account.SnapshotCoversModel("gemini-3.6-flash"))
	require.True(t, account.SnapshotCoversModel("gemini-3.6-flash-tiered"))
	require.False(t, account.SnapshotCoversModel("gemini-3.7-flash"))
}

func TestSupportsUpstreamModelSync_SkipsShadowAndUnknown(t *testing.T) {
	parentID := int64(9)
	require.False(t, supportsUpstreamModelSync(&Account{Platform: PlatformAntigravity, ParentAccountID: &parentID}))
	require.False(t, supportsUpstreamModelSync(&Account{Platform: "unknown"}))
	require.True(t, supportsUpstreamModelSync(&Account{Platform: PlatformAntigravity}))
	require.True(t, supportsUpstreamModelSync(&Account{Platform: PlatformKimi}))
}
