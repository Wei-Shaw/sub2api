package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamBillingRateSnapshotItemsPreservesOrderAndDropsMalformedSnapshots(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	accounts := []Account{
		{
			ID:       12,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey: map[string]any{
					"status":          UpstreamBillingProbeStatusOK,
					"last_attempt_at": now.Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).Format(time.RFC3339Nano),
					"data": map[string]any{
						"resolved_rate_multiplier": 2.5,
					},
				},
				UpstreamIdentityExtraKey: map[string]any{
					"detector_version": UpstreamIdentityDetectorVersion,
					"status":           UpstreamIdentityStatusIdentified,
					"provider":         UpstreamIdentityProviderNewAPI,
					"variant":          UpstreamIdentityVariantModern,
					"detected_at":      now.Format(time.RFC3339Nano),
				},
			},
		},
		{ID: 7, Extra: nil},
		{
			ID: 3,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey: map[string]any{"status": "legacy-invalid"},
			},
		},
		{
			ID:       5,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				UpstreamBillingProbeExtraKey: map[string]any{
					"status":          UpstreamBillingProbeStatusOK,
					"last_attempt_at": now.Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).Format(time.RFC3339Nano),
				},
			},
		},
	}

	items := BuildUpstreamBillingRateSnapshotItems(accounts)

	require.Len(t, items, 4)
	require.Equal(t, int64(12), items[0].AccountID)
	require.NotNil(t, items[0].Snapshot)
	require.Equal(t, UpstreamBillingProbeStatusOK, items[0].Snapshot.Status)
	require.NotNil(t, items[0].Identity)
	require.Equal(t, UpstreamIdentityVariantModern, items[0].Identity.Variant)
	require.Equal(t, int64(7), items[1].AccountID)
	require.Nil(t, items[1].Snapshot)
	require.Nil(t, items[1].Identity)
	require.Equal(t, int64(3), items[2].AccountID)
	require.Nil(t, items[2].Snapshot)
	require.Equal(t, int64(5), items[3].AccountID)
	require.Nil(t, items[3].Snapshot)
}
