//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_AdminUpdate_NoOwnerCheck(t *testing.T) {
	repo := &apiKeyRepoStub{
		apiKey: &APIKey{
			ID: 9, UserID: 100, Key: "sk-test-key", Status: StatusAPIKeyActive,
			IPWhitelist: []string{"10.0.0.1"},
		},
	}
	svc := &APIKeyService{apiKeyRepo: repo}
	status := StatusAPIKeyDisabled
	ips := []string{"10.0.0.1", "203.0.113.5"}
	out, err := svc.AdminUpdate(context.Background(), 9, AdminUpdateAPIKeyRequest{
		Status:      &status,
		IPWhitelist: &ips,
	})
	require.NoError(t, err)
	require.Equal(t, StatusAPIKeyDisabled, out.Status)
	require.Equal(t, ips, out.IPWhitelist)
	require.Len(t, repo.updatedKeys, 1)
	require.Equal(t, StatusAPIKeyDisabled, repo.updatedKeys[0].Status)
}
