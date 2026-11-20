package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/modelcatalog"
	"github.com/stretchr/testify/require"
)

func TestPluginCatalogSelectionHostPaths(t *testing.T) {
	cfg, err := modelcatalog.Parse([]byte(`{
		"library":[{"slug":"b","kind":"chat","display_name":"Beta","image_edit":false},{"slug":"pixel","kind":"image"}],
		"presets":[{"id":"p","name":"Preset","chat_models":["b","gpt-future-test","a","alias","unrouted","group-denied","account-denied"],"image_models":["pixel"],"default_chat_model":"gpt-future-test"}],
		"group_catalogs":[{"group_id":2,"preset_id":"p"},{"group_id":3,"chat_models":[]},{"group_id":4,"chat_models":["a"]}]
	}`))
	require.NoError(t, err)
	cfg.Models = testCatalogConfig(t).Models
	cfg.Models[0].MaxClientVersion = "0.164.0"
	manager := &PluginManager{}
	manager.catalog.Store(&pluginCatalogSnapshot{config: cfg})
	mapping := map[string]any{"b": "b", "a": "a", "alias": "pixel", "pixel": "pixel", "gpt-future-test": "gpt-future-test", "group-denied": "group-denied", "native": "native"}
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": mapping}}
	repo := codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{2: {account}, 4: {account}, 5: {account}}}
	s := &OpenAIGatewayService{pluginManager: manager, accountRepo: repo}
	group := &Group{ID: 2, Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"a", "alias", "gpt-future-test", "account-denied", "b", "unrouted", "pixel"}}}
	for _, tt := range []struct {
		version string
		want    []string
		hint    string
	}{
		{"0.164.0-alpha.1", []string{"b", "gpt-future-test", "a"}, "gpt-future-test"},
		{"0.159.9", []string{"b", "a"}, ""}, {"0.164.1", []string{"b", "a"}, ""}, {"", []string{"b", "a"}, ""},
	} {
		resp, handled, err := s.BuildGroupConfiguredCodexModelsManifest(context.Background(), group, "", tt.version)
		require.NoError(t, err)
		require.True(t, handled)
		var r struct {
			Models []struct {
				Slug     string
				Priority int
			}
			Hint string `json:"default_chat_model"`
		}
		require.NoError(t, json.Unmarshal(resp.Body, &r))
		var got []string
		for i, model := range r.Models {
			got = append(got, model.Slug)
			require.Equal(t, i, model.Priority)
		}
		require.Equal(t, tt.want, got)
		require.Equal(t, tt.hint, r.Hint)
		require.Contains(t, string(resp.Body), `"display_name":"Beta"`)
	}
	resp, handled, err := s.BuildGroupConfiguredCodexModelsManifest(context.Background(), &Group{ID: 3, Platform: PlatformOpenAI}, "", "1.0.0")
	require.NoError(t, err)
	require.True(t, handled)
	require.JSONEq(t, `{"models":[]}`, string(resp.Body))
	source := []string{"native", "group-denied", "account-denied", "unrouted", "pixel", "alias", "a", "gpt-future-test", "b"}
	for _, tc := range []struct {
		group   *Group
		want    []string
		handled bool
	}{
		{group, []string{"b", "a"}, true},
		{&Group{ID: 3, Platform: PlatformOpenAI}, []string{}, true},
		{&Group{ID: 4, Platform: PlatformOpenAI}, []string{"a"}, true},
		{&Group{ID: 5, Platform: PlatformOpenAI}, nil, false},
	} {
		got, handled, err := s.FilterPluginCatalogModelIDs(context.Background(), tc.group, source)
		require.NoError(t, err)
		require.Equal(t, tc.handled, handled)
		require.Equal(t, tc.want, got)
	}
	empty, handled, err := s.FilterPluginCatalogModelIDs(context.Background(), group, []string{})
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []string{}, empty)
	group.AllowImageGeneration = true
	withImage, handled, err := s.FilterPluginCatalogModelIDs(context.Background(), group, source)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []string{"b", "a", "pixel"}, withImage)
}

func TestPluginCatalogSelectionInheritsCapabilities(t *testing.T) {
	c, err := modelcatalog.Parse([]byte(`{"library":[{"slug":"narrow","kind":"chat","input_modalities":["text","audio"],"image_generation":true,"image_edit":false},{"slug":"inherit","kind":"chat","description":"new"}],"group_catalogs":[{"group_id":2,"chat_models":["inherit","narrow"]}]}`))
	require.NoError(t, err)
	src := []byte(`{"models":[{"slug":"narrow","image_generation":false,"image_edit":true,"input_modalities":["text","image"],"supports_image_detail_original":true},{"slug":"inherit","image_edit":true,"input_modalities":["image"],"unknown":{"tool":true}}],"upstream":true}`)
	got, _, err := newCatalogPolicy(c, 2, "").project(src, true, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"inherit","description":"new","image_edit":true,"input_modalities":["image"],"unknown":{"tool":true},"priority":0},{"slug":"narrow","image_generation":false,"image_edit":false,"input_modalities":["text"],"supports_image_detail_original":false,"priority":1}],"upstream":true}`, string(got))
}

func TestPluginCatalogSelectionPinnedListETag(t *testing.T) {
	s := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"a","owned_by":"provider"},{"id":"b"},{"id":"pixel"},{"id":"native"}]}`), nil
	}})
	a := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	a.Status, a.Schedulable = StatusActive, true
	s.accountRepo = splitCodexModelsAccountRepo{all: map[int64][]Account{2: {*a}}}
	s.pluginManager = &PluginManager{}
	group := &Group{ID: 2, Platform: PlatformOpenAI, AllowImageGeneration: true, CodexModelsManifestConfig: GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{a.ID}}}
	source, _, err := s.FetchPinnedOpenAIModelsList(context.Background(), group, 1, "")
	require.NoError(t, err)
	c, err := modelcatalog.Parse([]byte(`{"group_catalogs":[{"group_id":2,"chat_models":["b","a","not-upstream"],"image_models":["pixel"]}]}`))
	require.NoError(t, err)
	s.pluginManager.catalog.Store(&pluginCatalogSnapshot{config: c})
	got, _, err := s.FetchPinnedOpenAIModelsList(context.Background(), group, 1, source.ETag)
	require.NoError(t, err)
	require.False(t, got.NotModified)
	require.NotEqual(t, source.ETag, got.ETag)
	var list struct {
		Data []struct {
			ID    string
			Owner string `json:"owned_by"`
		}
	}
	require.NoError(t, json.Unmarshal(got.Body, &list))
	require.Len(t, list.Data, 3)
	require.Equal(t, "b", list.Data[0].ID)
	require.Equal(t, "a", list.Data[1].ID)
	require.Equal(t, "provider", list.Data[1].Owner)
	require.Equal(t, "pixel", list.Data[2].ID)
	cached, _, err := s.FetchPinnedOpenAIModelsList(context.Background(), group, 1, got.ETag)
	require.NoError(t, err)
	require.True(t, cached.NotModified)
	group.AllowImageGeneration = false
	denied, _, err := s.FetchPinnedOpenAIModelsList(context.Background(), group, 1, got.ETag)
	require.NoError(t, err)
	require.False(t, denied.NotModified)
	require.NotContains(t, string(denied.Body), "pixel")
}

type selectionInventoryGroups struct {
	GroupRepository
	group Group
}

func (r selectionInventoryGroups) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return []Group{r.group}, nil
}

func TestPluginCatalogSelectionAdminImages(t *testing.T) {
	raw := `{"library":[{"slug":"canvas","kind":"image","display_name":"Canvas","image_edit":false}],"group_catalogs":[{"group_id":2,"chat_models":["text"],"image_models":["canvas","gpt-image-2","not-routed","not-allowed"]}]}`
	i := &PluginInstallation{ID: 8, ConfigEncrypted: "ENC:" + raw, Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog}}}}
	a := Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{
			"text": "text", "gpt-image-2": "gpt-image-2", "canvas": "gpt-image-2", "not-allowed": "gpt-image-2",
			"not-published": "gpt-image-2",
		}},
	}
	group := Group{
		ID: 2, Platform: PlatformOpenAI, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"text", "canvas", "gpt-image-2", "not-published"}},
	}
	m := &PluginManager{repo: &pluginConfigRepository{installation: i}, encryptor: pluginTokenEncryptor{},
		catalogGroups: selectionInventoryGroups{group: group}, catalogAccounts: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{2: {a}}}}
	got, err := m.CatalogInventory(context.Background(), 8, 2)
	require.NoError(t, err)
	require.Len(t, got.Models, 1)
	require.Contains(t, string(got.Models[0].Descriptor), `"slug":"text"`)
	require.Len(t, got.ImageModels, 2)
	require.Contains(t, string(got.ImageModels[0].Descriptor), `"display_name":"Canvas"`)
	require.Contains(t, string(got.ImageModels[0].Descriptor), `"image_edit":false`)
	require.Contains(t, string(got.ImageModels[1].Descriptor), `"slug":"gpt-image-2"`)
	discovered := map[string]string{}
	for _, row := range got.UpstreamImageModels {
		var d struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
		}
		require.NoError(t, json.Unmarshal(row.Descriptor, &d))
		discovered[d.Slug] = d.DisplayName
		require.Equal(t, "upstream", row.Source)
	}
	require.Equal(t, map[string]string{"canvas": "canvas", "gpt-image-2": "gpt-image-2", "not-published": "not-published"}, discovered)
	cfg, err := modelcatalog.Parse([]byte(raw))
	require.NoError(t, err)
	m.catalog.Store(&pluginCatalogSnapshot{config: cfg})
	s := &OpenAIGatewayService{pluginManager: m, accountRepo: m.catalogAccounts}
	list, handled, err := s.FilterPluginCatalogModelIDs(context.Background(), &group, []string{"text", "gpt-image-2", "canvas", "not-published", "not-allowed"})
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []string{"text", "canvas", "gpt-image-2"}, list)
	group.AllowImageGeneration = false
	m.catalogGroups = selectionInventoryGroups{group: group}
	got, err = m.CatalogInventory(context.Background(), 8, 2)
	require.NoError(t, err)
	require.Empty(t, got.ImageModels)
	require.Empty(t, got.UpstreamImageModels)
}
