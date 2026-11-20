package service

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/modelcatalog"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func testCatalogConfig(t *testing.T) *modelcatalog.Config {
	t.Helper()
	c, err := modelcatalog.Parse([]byte(`{"schema_version":1,"models":[{"group_ids":[2],"min_client_version":"0.160.0","descriptor":{"slug":"gpt-future-test","display_name":"Future","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"medium"},{"effort":"max"}],"base_instructions":"future instructions","shell_type":"unified_exec","visibility":"list","future_tool":{"version":5},"use_responses_lite":true}}]}`))
	require.NoError(t, err)
	return c
}

func TestPluginCatalogManifestUsesIsolatedCapability(t *testing.T) {
	manifest := PluginManifest{
		SchemaVersion: 1,
		ID:            "local.test.catalog",
		Name:          "Model catalog",
		Version:       "1.0.0",
		Requires: PluginRequirements{
			Sub2API: ">=0.1.0", PluginProtocol: pluginv1.ProtocolVersion,
			TransportAPI: pluginv1.TransportAPIVersion, UIBridge: 2,
		},
		Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog, Platform: PlatformOpenAI, AccountType: "catalog"}},
		Runtimes:     map[string]PluginRuntime{runtime.GOOS + "-" + runtime.GOARCH: {Path: "bin/catalog"}},
		UI:           PluginUIManifest{Entrypoint: "ui/index.html"},
		Files: map[string]string{
			"bin/catalog":   strings.Repeat("a", 64),
			"ui/index.html": strings.Repeat("b", 64),
		},
	}
	require.NoError(t, manifest.Validate())

	transportAndCatalog := manifest
	transportAndCatalog.Capabilities = append(append([]PluginCapability(nil), manifest.Capabilities...), PluginCapability{
		ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth,
	})
	require.Error(t, transportAndCatalog.Validate())

	transportBridgeV2 := manifest
	transportBridgeV2.Capabilities = []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}}
	require.Error(t, transportBridgeV2.Validate())
}

func TestPluginCatalogPreservesMetadataWithoutWideningVisibility(t *testing.T) {
	m := &PluginManager{}
	m.catalog.Store(&pluginCatalogSnapshot{pluginID: 7, config: testCatalogConfig(t)})
	body := []byte(`{"models":[{"slug":"gpt-future-test","use_responses_lite":false},{"slug":"gpt-other","base_instructions":"other"}],"future_envelope":42}`)
	out, changed, err := m.overlayCodexCatalog(body, 2, "0.164.0-alpha.11.1")
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(out), `"future_tool":{"version":5}`)
	require.Contains(t, string(out), `"use_responses_lite":false`)
	require.Contains(t, string(out), `"future_envelope":42`)
	var root struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(out, &root))
	require.Len(t, root.Models, 2)
	for _, test := range []struct {
		group   int64
		version string
	}{{3, "0.164.0"}, {2, "0.159.0"}, {2, ""}} {
		out, changed, err = m.overlayCodexCatalog(body, test.group, test.version)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}
	_, changed, err = m.overlayCodexCatalog([]byte(`{"models":[]}`), 2, "0.164.0")
	require.NoError(t, err)
	require.False(t, changed)
}
func TestPluginCatalogRegistersOnlyModelsSupportedByExistingRoutes(t *testing.T) {
	m := &PluginManager{}
	m.catalog.Store(&pluginCatalogSnapshot{config: testCatalogConfig(t)})
	a := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-other": "gpt-other"}}}
	require.Empty(t, m.appendCatalogModelIDs(nil, []Account{a}, &Group{ID: 2}, "0.164.0"))
	a.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-future-*": "gpt-future-*"}}
	require.Equal(t, []string{"gpt-future-test"}, m.appendCatalogModelIDs(nil, []Account{a}, &Group{ID: 2}, "0.164.0"))
	require.Empty(t, m.appendCatalogModelIDs(nil, []Account{a}, &Group{ID: 3}, "0.164.0"))
	require.Empty(t, pluginAccountScopeFromManifest(PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog, Platform: PlatformOpenAI, AccountType: "catalog"}}}).entries)
}

func TestPluginCatalogExplicitGroupsAndClientOrder(t *testing.T) {
	raw := []byte(`{"models":[{"slug":"native"},{"slug":"second"},{"slug":"hidden"},{"slug":"first"}],"extension":true}`)
	c := &modelcatalog.Config{SchemaVersion: 1, ExplicitOnlyGroupIDs: []int64{2}}
	first, second := int32(-5), int32(25)
	for _, row := range []struct {
		slug  string
		order *int32
	}{{"first", &first}, {"second", &second}, {"hidden", nil}, {"absent", nil}} {
		c.Models = append(c.Models, modelcatalog.Model{
			GroupIDs: []int64{2, 4}, MinClientVersion: "1.0.0", Hidden: row.slug == "hidden",
			ClientOrder: row.order, Descriptor: json.RawMessage(`{"slug":"` + row.slug + `","input_modalities":["text","image"],"supports_image_detail_original":true}`),
		})
	}
	manager := &PluginManager{}
	manager.catalog.Store(&pluginCatalogSnapshot{config: c})
	for _, tt := range []struct {
		group   int64
		version string
		want    []string
	}{{2, "1.0.0", []string{"first", "second"}}, {2, "0.9.0", []string{}}, {4, "1.0.0", []string{"first", "second", "native"}}} {
		body, changed, err := manager.overlayCodexCatalog(raw, tt.group, tt.version)
		require.NoError(t, err)
		require.True(t, changed)
		var result struct {
			Models []struct {
				Slug     string
				Priority *int
				Input    []string `json:"input_modalities"`
				Original bool     `json:"supports_image_detail_original"`
			}
			Extension bool
		}
		require.NoError(t, json.Unmarshal(body, &result))
		require.True(t, result.Extension)
		require.NotNil(t, result.Models)
		require.Len(t, result.Models, len(tt.want))
		for i, model := range result.Models {
			require.Equal(t, tt.want[i], model.Slug)
			require.NotNil(t, model.Priority)
			require.Equal(t, i, *model.Priority)
			if model.Slug != "native" {
				require.Equal(t, []string{"text", "image"}, model.Input)
				require.True(t, model.Original)
			}
		}
	}
	body, changed, err := manager.overlayCodexCatalog(raw, 3, "1.0.0")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, raw, body)
}

type failingCatalogConfigRepo struct{ pluginConfigRepository }

type catalogConfigClient struct{ normalizingPluginClient }

func (c *catalogConfigClient) ValidateConfig(_ context.Context, r *pluginv1.ValidateConfigRequest, _ ...grpc.CallOption) (*pluginv1.ValidateConfigResponse, error) {
	return &pluginv1.ValidateConfigResponse{Valid: true, NormalizedConfigJson: r.ConfigJson}, nil
}

func (r *failingCatalogConfigRepo) UpdateConfig(context.Context, int64, string, string) error {
	return errors.New("database unavailable")
}
func TestPluginCatalogFailedSaveKeepsPublishedSnapshot(t *testing.T) {
	c := testCatalogConfig(t)
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	i := &PluginInstallation{ID: 9, BinarySHA256: strings.Repeat("a", 64), ConfigEncrypted: "ENC:" + string(raw), Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog}}}, Bindings: []PluginBinding{{Capability: PluginCapabilityCodexModelCatalog, Platform: PlatformOpenAI, AccountType: "catalog", Enabled: true}}}
	repo := &failingCatalogConfigRepo{pluginConfigRepository: pluginConfigRepository{installation: i}}
	client := &catalogConfigClient{}
	m := &PluginManager{repo: repo, encryptor: pluginTokenEncryptor{}, runtimes: map[int64]*pluginRuntime{9: {installation: i, api: client}}}
	snapshot := &pluginCatalogSnapshot{pluginID: 9, config: c}
	m.catalog.Store(snapshot)
	_, err = m.SaveConfig(context.Background(), 9, []byte(`{"schema_version":1,"models":[]}`))
	require.ErrorContains(t, err, "database unavailable")
	require.Same(t, snapshot, m.catalog.Load())
	require.JSONEq(t, string(raw), string(client.applied))
}

type concurrentCatalogConfigRepo struct {
	*pluginConfigRepository
	latestConfig string
}

func (r *concurrentCatalogConfigRepo) UpdateConfigIfCurrent(_ context.Context, _ int64, _, expectedBinarySHA256, expectedConfigEncrypted string) error {
	if expectedBinarySHA256 != r.installation.BinarySHA256 {
		return ErrPluginStateChanged
	}
	if expectedConfigEncrypted != r.installation.ConfigEncrypted {
		return ErrPluginConfigChanged
	}
	r.installation.ConfigEncrypted = "ENC:" + r.latestConfig
	return ErrPluginConfigChanged
}

func TestPluginCatalogSaveRestoresConcurrentRevision(t *testing.T) {
	current := []byte(`{"schema_version":1,"models":[]}`)
	candidate := []byte(`{"schema_version":1,"models":[],"explicit_only_group_ids":[3]}`)
	latest := []byte(`{"schema_version":1,"models":[],"explicit_only_group_ids":[2]}`)
	installation := &PluginInstallation{
		ID: 9, BinarySHA256: strings.Repeat("a", 64), ConfigEncrypted: "ENC:" + string(current),
		Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog}}},
		Bindings: []PluginBinding{{Capability: PluginCapabilityCodexModelCatalog, Platform: PlatformOpenAI, AccountType: "catalog", Enabled: true}},
	}
	repo := &concurrentCatalogConfigRepo{pluginConfigRepository: &pluginConfigRepository{installation: installation}, latestConfig: string(latest)}
	client := &catalogConfigClient{}
	manager := &PluginManager{
		repo: repo, encryptor: pluginTokenEncryptor{},
		runtimes: map[int64]*pluginRuntime{9: {installation: installation, api: client}},
	}
	currentCatalog, err := modelcatalog.Parse(current)
	require.NoError(t, err)
	manager.catalog.Store(&pluginCatalogSnapshot{pluginID: 9, config: currentCatalog})

	_, err = manager.SaveConfigIfCurrent(context.Background(), 9, candidate, PluginConfigETag(current))
	require.ErrorIs(t, err, ErrPluginConfigChanged)
	require.JSONEq(t, string(latest), string(client.applied))
	require.Equal(t, "ENC:"+string(latest), manager.runtimes[9].installation.ConfigEncrypted)
	require.True(t, manager.catalog.Load().config.ExplicitOnly(2))
	require.False(t, manager.catalog.Load().config.ExplicitOnly(3))
}

func TestPluginCatalogLifecycleCannotClearOAuthRoute(t *testing.T) {
	m := &PluginManager{runtimes: map[int64]*pluginRuntime{}}
	route := &pluginRoute{pluginID: 1, rolloutPercent: 100}
	m.route.Store(route)
	i := &PluginInstallation{ID: 2, Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityCodexModelCatalog}}}}
	m.publishRuntimeLocked(i, &pluginRuntime{installation: i})
	require.Same(t, route, m.route.Load())
	require.Empty(t, m.detachAllRuntimes())
	require.NotNil(t, m.runtimes[2])
	m.route.Store(route)
	m.removeRuntimeLocked(2)
	require.Same(t, route, m.route.Load())
	require.Nil(t, m.catalog.Load())
}
