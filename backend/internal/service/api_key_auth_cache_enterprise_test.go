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
