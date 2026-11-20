package service

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/pkg/modelcatalog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPluginCatalogHideRestoreRespectsScope(t *testing.T) {
	config, err := modelcatalog.Parse([]byte(`{"schema_version":1,"models":[{"group_ids":[2],"hidden":true,"min_client_version":"0.160.0","descriptor":{"slug":"gpt-6-sol"}}]}`))
	require.NoError(t, err)
	manager := &PluginManager{}
	manager.catalog.Store(&pluginCatalogSnapshot{config: config})
	body := []byte(`{"models":[{"slug":"gpt-6-sol","future_field":123},{"slug":"gpt-other"}],"future_envelope":42}`)
	out, changed, err := manager.overlayCodexCatalog(body, 2, "0.164.0")
	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, string(out), "gpt-6-sol")
	require.Contains(t, string(out), "gpt-other")
	require.Contains(t, string(out), "future_envelope")
	for _, scope := range []struct {
		group   int64
		version string
	}{{3, "0.164.0"}, {2, "0.159.0"}, {2, ""}} {
		out, changed, err = manager.overlayCodexCatalog(body, scope.group, scope.version)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}
	account := Account{Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"*": "*"}}}
	require.Empty(t, manager.appendCatalogModelIDs(nil, []Account{account}, &Group{ID: 2}, "0.164.0"))
	config.Models = nil
	out, changed, err = manager.overlayCodexCatalog(body, 2, "0.164.0")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, out)
}
func TestPluginCatalogHidingLastModelReturnsEmptyArray(t *testing.T) {
	config, err := modelcatalog.Parse([]byte(`{"schema_version":1,"models":[{"group_ids":[2],"hidden":true,"descriptor":{"slug":"gpt-6-sol"}}]}`))
	require.NoError(t, err)
	manager := &PluginManager{}
	manager.catalog.Store(&pluginCatalogSnapshot{config: config})
	out, changed, err := manager.overlayCodexCatalog([]byte(`{"models":[{"slug":"gpt-6-sol"}]}`), 2, "0.164.0")
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"models":[]}`, string(out))
}

type catalogInventoryGroupRepo struct{ GroupRepository }

func TestPluginCatalogRejectsStaleRevision(t *testing.T) {
	live := `{"models":[]}`
	installation := &PluginInstallation{ID: 9, ConfigEncrypted: "ENC:" + live}
	manager := &PluginManager{repo: &pluginConfigRepository{installation: installation}, encryptor: pluginTokenEncryptor{}}
	_, err := manager.SaveConfigIfCurrent(context.Background(), 9, []byte(`{}`), PluginConfigETag([]byte(`{}`)))
	require.ErrorIs(t, err, ErrPluginConfigChanged)
	got, err := manager.GetConfig(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, live, string(got))
}

func (*catalogInventoryGroupRepo) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return []Group{{ID: 2, Name: "Production", Platform: PlatformOpenAI}}, nil
}

type catalogInventoryAccountRepo struct{ AccountRepository }

func (*catalogInventoryAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return catalogInventoryAccounts(), nil
}
func (*catalogInventoryAccountRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	return catalogInventoryAccounts(), nil
}
func catalogInventoryAccounts() []Account {
	return []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "NEVER_EXPOSE_CATALOG_CREDENTIAL", "base_url": "https://private-upstream.invalid", "model_mapping": map[string]any{"gpt-6-sol": "gpt-6-sol", "custom-alias": "gpt-6-sol", "gpt-image-2": "gpt-image-2"}}}}
}
func TestPluginCatalogInventoryUsesExistingSourcesWithoutSecrets(t *testing.T) {
	installation := &PluginInstallation{ID: 9, Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog}}}}
	manager := &PluginManager{repo: &pluginConfigRepository{installation: installation}, catalogAccounts: &catalogInventoryAccountRepo{}, catalogGroups: &catalogInventoryGroupRepo{}}
	inventory, err := manager.CatalogInventory(context.Background(), 9, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), inventory.GroupID)
	require.Len(t, inventory.Groups, 1)
	require.Len(t, inventory.Models, 2)
	raw, err := json.Marshal(inventory)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "NEVER_EXPOSE_CATALOG_CREDENTIAL")
	require.NotContains(t, string(raw), "private-upstream")
	require.NotContains(t, string(raw), "gpt-image-2")
	sources := map[string]string{}
	for _, model := range inventory.Models {
		var descriptor map[string]any
		require.NoError(t, json.Unmarshal(model.Descriptor, &descriptor))
		slug, ok := descriptor["slug"].(string)
		require.True(t, ok)
		sources[slug] = model.Source
		require.NotEmpty(t, descriptor["model_messages"])
	}
	require.Equal(t, "builtin", sources["gpt-6-sol"])
	require.Equal(t, "upstream", sources["custom-alias"])
	_, err = manager.CatalogInventory(context.Background(), 9, 999)
	require.ErrorIs(t, err, ErrGroupNotFound)
	installation.Manifest.Capabilities = nil
	_, err = manager.CatalogInventory(context.Background(), 9, 2)
	require.Error(t, err)
}

func TestPluginCatalogV2193DiscoveryAndCacheLifecycle(t *testing.T) {
	manager := &PluginManager{}
	catalog := testCatalogConfig(t)
	manager.catalog.Store(&pluginCatalogSnapshot{pluginID: 9, config: catalog})
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6-sol": "gpt-6-sol", "gpt-future-*": "gpt-future-*"}}}
	svc := &OpenAIGatewayService{pluginManager: manager, accountRepo: codexModelsVisibilityAccountRepo{
		byGroup: map[int64][]Account{2: {account}, 3: {account}},
	}}
	group := &Group{ID: 2, Platform: PlatformOpenAI}
	manifest, handled, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "", "0.164.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.Contains(t, string(manifest.Body), `"slug":"gpt-future-test"`)
	require.Contains(t, string(manifest.Body), `"future_tool":{"version":5}`)
	require.Contains(t, string(manifest.Body), "future instructions")
	require.Contains(t, string(manifest.Body), `"use_responses_lite":false`)
	cached, handled, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, manifest.ETag, "0.164.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, cached.NotModified)
	other, handled, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), &Group{ID: 3, Platform: PlatformOpenAI}, "", "0.164.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.NotContains(t, string(other.Body), `"slug":"gpt-future-test"`)
	hidden := *catalog
	hidden.Models = append(hidden.Models[:0:0], catalog.Models...)
	hidden.Models[0].Hidden = true
	manager.catalog.Store(&pluginCatalogSnapshot{pluginID: 9, config: &hidden})
	updated, handled, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, manifest.ETag, "0.164.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.False(t, updated.NotModified)
	require.NotEqual(t, manifest.ETag, updated.ETag)
	require.NotContains(t, string(updated.Body), `"slug":"gpt-future-test"`)
	manager.catalog.Store(&pluginCatalogSnapshot{pluginID: 9, config: catalog})
	restored, handled, err := svc.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "", "0.164.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, manifest.ETag, restored.ETag)
}
