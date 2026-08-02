//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotPreservesOrganizationSubscriptionID(t *testing.T) {
	organizationSubscriptionID := int64(501)
	svc := &APIKeyService{}
	key := &APIKey{
		ID: 1, UserID: 2, OrganizationSubscriptionID: &organizationSubscriptionID,
		User: &User{ID: 2, Status: StatusActive},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	restored := svc.snapshotToAPIKey("secret", snapshot)

	require.NotNil(t, restored.OrganizationSubscriptionID)
	require.Equal(t, organizationSubscriptionID, *restored.OrganizationSubscriptionID)
}

func TestAPIKeyAuthSnapshotPreservesFallbackGroupOrder(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{
		ID: 1, UserID: 2, FallbackGroupIDs: []int64{9, 3, 7},
		User: &User{ID: 2, Status: StatusActive},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
	require.Equal(t, []int64{9, 3, 7}, snapshot.FallbackGroupIDs)

	restored := svc.snapshotToAPIKey("secret", snapshot)
	require.Equal(t, []int64{9, 3, 7}, restored.FallbackGroupIDs)

	restored.FallbackGroupIDs[0] = 100
	require.Equal(t, []int64{9, 3, 7}, snapshot.FallbackGroupIDs, "snapshot and restored key must not alias")
}
