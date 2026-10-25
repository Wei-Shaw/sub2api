//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetByKeyForAuthCarriesGroupForceOpenAIFast(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("fast-proj-group-%d", suffix), Platform: service.PlatformOpenAI,
		RateMultiplier: 1, ForceOpenAIFast: true, FreeOpenAIFast: true,
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("fast-proj-%d@example.com", suffix), Concurrency: 5,
	})
	groupID := group.ID
	keyValue := fmt.Sprintf("sk-fast-proj-%d", suffix)
	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	key := &service.APIKey{UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "fast-proj", Status: service.StatusActive}
	require.NoError(t, apiKeyRepo.Create(ctx, key))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, deleteAuthCacheOutboxByKeySQL(), keyValue)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, deleteByIDSQL("api_keys"), key.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, deleteByIDSQL("users"), user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, deleteByIDSQL("groups"), group.ID)
		require.NoError(t, err)
	})

	got, err := apiKeyRepo.GetByKeyForAuth(ctx, keyValue)
	require.NoError(t, err)
	require.NotNil(t, got.Group)
	require.True(t, got.Group.ForceOpenAIFast)
	require.True(t, got.Group.FreeOpenAIFast)
}
