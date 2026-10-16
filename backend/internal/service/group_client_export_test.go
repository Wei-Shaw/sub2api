//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGroupClientExportOptions prevents disallowed models or another group's
// account models from becoming defaults through the administrator picker.
func TestGroupClientExportOptions(t *testing.T) {
	group := &Group{Platform: PlatformOpenAI, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.6-sol", "gpt-image-1", "alias-*"}}}
	accounts := []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol", "gpt-5.6-luna": "gpt-5.6-luna", "gpt-image-1": "gpt-image-1", "alias-image": "gpt-image-1", "wild-*": "gpt-5.6-sol"}}}, {ID: 2, Platform: PlatformAnthropic, Credentials: map[string]any{"model_mapping": map[string]any{"claude-private": "claude-sonnet-4-6"}}}}
	options := buildGroupClientExportOptions(group, accounts, nil)
	require.Equal(t, []string{"gpt-5.6-sol"}, options.CodexModels)
	require.Empty(t, options.GeminiModels)
	require.Empty(t, options.GrokModels)
}

// TestGroupClientExportNativeSlots ensures each platform exposes only fields
// supported by its native templates, including dedicated fixed templates.
func TestGroupClientExportNativeSlots(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok, PlatformTypeSafe, PlatformDeepseek, PlatformMiniMax, PlatformAnthropic, PlatformComposite} {
		t.Run(platform, func(t *testing.T) {
			options := buildGroupClientExportOptions(&Group{Platform: platform}, nil, nil)
			if platform == PlatformTypeSafe {
				require.Empty(t, options.CodexModels)
			} else {
				require.NotEmpty(t, options.CodexModels)
			}
			require.Equal(t, platform == PlatformGemini || platform == PlatformAntigravity, len(options.GeminiModels) > 0)
			require.Equal(t, platform == PlatformGrok, len(options.GrokModels) > 0)
		})
	}
}

// TestGroupClientExportPinnedEmpty never replaces a missing pinned catalog with
// an unrelated built-in model or another account in the group.
func TestGroupClientExportPinnedEmpty(t *testing.T) {
	group := &Group{Platform: PlatformOpenAI, CodexModelsManifestConfig: GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{99}}}
	options := buildGroupClientExportOptions(group, []Account{{ID: 1, Platform: PlatformOpenAI}}, nil)
	require.Empty(t, options.CodexModels)
}

// TestGroupClientExportCompositeRoutes preserves any/responses routes and
// pass-through model names while excluding media and incompatible endpoints.
func TestGroupClientExportCompositeRoutes(t *testing.T) {
	routes := []CompositeModelRoute{
		{PublicModel: "custom-text", MatchType: CompositeRouteMatchExact, Endpoint: CompositeRouteEndpointAny, TargetPlatform: PlatformOpenAI, Enabled: true},
		{PublicModel: "text-alias", UpstreamModel: "gpt-5.6-sol", MatchType: CompositeRouteMatchExact, Endpoint: CompositeRouteEndpointResponses, TargetPlatform: PlatformOpenAI, Enabled: true},
		{PublicModel: "media-alias", UpstreamModel: "gpt-image-1", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenAI, Enabled: true},
		{PublicModel: "messages-only", UpstreamModel: "claude-sonnet-4-6", MatchType: CompositeRouteMatchExact, Endpoint: CompositeRouteEndpointMessages, TargetPlatform: PlatformAnthropic, Enabled: true},
	}
	options := buildGroupClientExportOptions(&Group{Platform: PlatformComposite, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"custom-text", "text-alias", "media-alias", "messages-only"}}}, nil, routes)
	require.Equal(t, []string{"custom-text", "text-alias"}, options.CodexModels)
}

// TestGroupClientExportValidation checks server enforcement independently of UI
// selections, including unsupported slots, wildcard values and injection input.
func TestGroupClientExportValidation(t *testing.T) {
	svc := &adminServiceImpl{}
	for _, cfg := range []GroupClientExportConfig{{CodexModel: "unknown-model"}, {CodexModel: "gpt-*"}, {CodexModel: "gpt-5.6-sol\"\nnetwork_access=\"enabled"}, {GeminiModel: "gemini-2.5-pro"}, {GrokModel: "grok-4.5"}} {
		group := &Group{Platform: PlatformOpenAI, ClientExportConfig: cfg}
		require.Error(t, svc.validateGroupClientExport(context.Background(), group))
	}
	for _, cfg := range []GroupClientExportConfig{{}, {CodexModel: "gpt-5.6-sol"}} {
		require.NoError(t, svc.validateGroupClientExport(context.Background(), &Group{Platform: PlatformOpenAI, ClientExportConfig: cfg}))
	}
}

// TestGroupClientExportPartialUpdate preserves omitted values and allows an
// explicit empty object to restore the native client defaults.
func TestGroupClientExportPartialUpdate(t *testing.T) {
	for _, reset := range []bool{false, true} {
		repo := &groupRepoStubForAdmin{getByID: &Group{ID: 42, Name: "test", Platform: PlatformOpenAI, SubscriptionType: "standard", ClientExportConfig: GroupClientExportConfig{CodexModel: "gpt-5.6-sol"}}}
		svc := &adminServiceImpl{groupRepo: repo}
		input := &UpdateGroupInput{Name: "renamed"}
		if reset {
			input.ClientExportConfig = &GroupClientExportConfig{}
		}
		_, err := svc.UpdateGroup(context.Background(), 42, input)
		require.NoError(t, err)
		if reset {
			require.Empty(t, repo.updated.ClientExportConfig.CodexModel)
		} else {
			require.Equal(t, "gpt-5.6-sol", repo.updated.ClientExportConfig.CodexModel)
		}
	}
}

// TestGroupClientExportPresets rejects unsupported settings and arbitrary values
// at the service boundary, including direct API requests bypassing the UI.
func TestGroupClientExportPresets(t *testing.T) {
	svc := &adminServiceImpl{}
	for _, tc := range []struct {
		platform string
		config   GroupClientExportConfig
		valid    bool
	}{
		{PlatformOpenAI, GroupClientExportConfig{CodexAuthMode: "api-key", CodexCatalogMode: "file", CodexReasoningEffort: "high", CCUsageMode: "disabled", CCUsageInterval: "15"}, true},
		{PlatformGemini, GroupClientExportConfig{CodexCatalogMode: "remote", CCUsageInterval: "0"}, true},
		{PlatformAnthropic, GroupClientExportConfig{ClaudeTraffic: "enabled"}, true},
		{PlatformOpenAI, GroupClientExportConfig{CodexAuthMode: "custom"}, false},
		{PlatformOpenAI, GroupClientExportConfig{CodexReasoningEffort: "arbitrary"}, false},
		{PlatformOpenAI, GroupClientExportConfig{CodexCatalogMode: "https://other.example"}, false},
		{PlatformOpenAI, GroupClientExportConfig{CCUsageInterval: "1"}, false},
		{PlatformOpenAI, GroupClientExportConfig{CCUsageMode: "false"}, false},
		{PlatformAnthropic, GroupClientExportConfig{ClaudeTraffic: "custom"}, false},
		{PlatformGemini, GroupClientExportConfig{ClaudeTraffic: "disabled"}, false},
		{PlatformGemini, GroupClientExportConfig{CodexAuthMode: "api-key"}, false},
		{PlatformTypeSafe, GroupClientExportConfig{CCUsageMode: "disabled"}, false},
		{PlatformTypeSafe, GroupClientExportConfig{CodexCatalogMode: "file"}, false},
	} {
		err := svc.validateGroupClientExport(context.Background(), &Group{Platform: tc.platform, ClientExportConfig: tc.config})
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
