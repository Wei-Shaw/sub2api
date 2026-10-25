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

func TestGetByKeyForAuthCarriesGroupCodexModelsManifestConfig(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("codex-manifest-proj-group-%d", suffix), Platform: service.PlatformOpenAI,
		RateMultiplier: 1,
		CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{
			Enabled:             true,
			AccountIDs:          []int64{11, 22},
			FallbackToScheduler: true,
		},
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("codex-manifest-proj-%d@example.com", suffix), Concurrency: 5,
	})
	groupID := group.ID
	keyValue := fmt.Sprintf("sk-codex-manifest-proj-%d", suffix)
	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	key := &service.APIKey{UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "codex-manifest-proj", Status: service.StatusActive}
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
	require.True(t, got.Group.CodexModelsManifestConfig.Enabled, "codex_models_manifest_config 必须进入认证投影（投影漏列会让固定账号模式静默失效）")
	require.Equal(t, []int64{11, 22}, got.Group.CodexModelsManifestConfig.AccountIDs)
	require.True(t, got.Group.CodexModelsManifestConfig.FallbackToScheduler)
}
