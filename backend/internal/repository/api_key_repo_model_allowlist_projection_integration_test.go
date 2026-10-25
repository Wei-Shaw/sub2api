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

func TestGetByKeyForAuthCarriesGroupModelAllowlist(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("model-allowlist-proj-group-%d", suffix), Platform: service.PlatformOpenAI,
		RateMultiplier: 1,
		ModelAllowlist: service.GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gpt-5.4", "gpt-5.5-*"},
		},
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("model-allowlist-proj-%d@example.com", suffix), Concurrency: 5,
	})
	groupID := group.ID
	keyValue := fmt.Sprintf("sk-model-allowlist-proj-%d", suffix)
	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	key := &service.APIKey{UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "model-allowlist-proj", Status: service.StatusActive}
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
	require.True(t, got.Group.ModelAllowlist.Enabled, "model_allowlist 必须进入认证投影（投影漏列会让分组级准入静默失效）")
	require.Equal(t, []string{"gpt-5.4", "gpt-5.5-*"}, got.Group.ModelAllowlist.Models)
}
